<template>
  <div class="task-admin-page vgpu-admin-page" :class="{ 'is-embedded': hideTitle }">
    <div v-if="!hideTitle" class="vgpu-admin-page-title">{{ $t('task.title') }}</div>

    <div class="task-admin-top-wrap" v-if="!hideTitle">
      <Top />
    </div>

    <div ref="tableWrap" class="task-admin-table-wrap">
      <toolbar
        v-model="eyeColumnKeys"
        :column-options="columnOptions"
        :refreshing="tableRefreshing"
        @refresh="refreshTable"
      >
        <div class="workload-filters">
          <SegmentedControl
            v-model="filters.status"
            class="workload-status-filter"
            :options="statusTabOptions"
            :aria-label="$t('task.status')"
            @change="applyFilters"
          />
          <t-select
            v-model="filters.nodeName"
            class="workload-filter-select"
            clearable
            :placeholder="$t('task.allNodes')"
            :options="nodeOptions"
            @change="onNodeNameChange"
          />
          <t-select
            v-model="filters.deviceId"
            class="workload-filter-select"
            clearable
            :placeholder="$t('task.allCards')"
            :options="cardOptions"
            @change="applyFilters"
          />
          <t-input
            v-model="filters.name"
            class="workload-search"
            clearable
            :placeholder="$t('task.searchWorkloadName')"
            @enter="applyFilters"
            @blur="applyFilters"
          >
            <template #prefix-icon>
              <search-icon :style="{ cursor: 'pointer' }" />
            </template>
          </t-input>
        </div>
      </toolbar>
      <stateful-table
        :status="tableStatus"
        :refreshing="tableRefreshing"
        :refresh-error="tableRefreshError"
        :has-rows="tableData.length > 0"
        :column-count="visibleColumns.length"
        @retry="refreshTable"
      >
        <t-table
          :key="locale"
          row-key="workloadRowKey"
          class="workload-table vgpu-table-skin"
          :data="tableData"
          :columns="visibleColumns"
          table-layout="auto"
          :style="style"
        />
        <template #footer>
          <table-pagination
            :total="pagination.total"
            :current="pagination.current"
            :page-size="pagination.pageSize"
            :page-sizes="pagination.pageSizeOptions"
            :show-jumper="pagination.showJumper"
            @change="changePage"
          />
        </template>
      </stateful-table>
    </div>
    <SchedulingDrawer
      :identity-pod="selectedSchedulingPod"
      :container-name="selectedSchedulingContainer"
      :focus-return-target="tableWrap"
      @close="selectedSchedulingPod = null"
      @updated="onSchedulingUpdated"
    />
  </div>
</template>

<script setup lang="jsx">
import taskApi from '~/vgpu/api/task';
import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import TablePagination from '@/components/TablePlus/Pagination.vue';
import StatefulTable from '@/components/TablePlus/StatefulTable.vue';
import EllipsisText from '@/components/EllipsisText.vue';
import { roundToDecimal, timeParse } from '@/utils';
import request from '@/utils/request';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { reactive, ref, computed, onBeforeUnmount, onMounted, toRefs, watch } from 'vue';
import { RouterLink, useRoute, useRouter } from 'vue-router';
import Top from './top.vue';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import useTableFilters from '~/vgpu/hooks/useTableFilters';
import { buildWorkloadDetailLocation, createWorkloadRowKey, formatWorkloadName } from './workload-identity.mjs';
import WorkloadStatus from './WorkloadStatus.vue';
import { getWorkloadStatusOptions } from './workload-status.mjs';
import { createRequestState, isLatestRequest, rejectRequest, REQUEST_STATUS, resolveRequest, startRequest } from '@/hooks/request-state.mjs';
import SchedulingDrawer from './SchedulingDrawer.vue';
import SegmentedControl from '@/components/SegmentedControl/index.vue';
import { getWorkloadRequestTotals } from './scheduling-display.mjs';

const props = defineProps(['hideTitle', 'filters', 'style']);
const { t, locale } = useI18n();
const tableWrap = ref(null);
const selectedSchedulingPod = ref(null);
const selectedSchedulingContainer = ref('');
const hasManualNodeScope = ref(false);
const filters = reactive({
  name: props.filters?.name || '',
  nodeName: props.filters?.nodeName,
  status: props.filters?.status || '',
  deviceId: props.filters?.deviceId,
});
const rawNodeNames = ref([]);
const rawCards = ref([]);
const nodeOptions = computed(() => [
  { label: t('task.allNodes'), value: undefined },
  ...rawNodeNames.value.map((name) => ({ label: name, value: name })),
]);
const cardOptions = computed(() => {
  // When a node is selected, only offer the cards that live on that node.
  const cards = filters.nodeName
    ? rawCards.value.filter((card) => card.nodeName === filters.nodeName)
    : rawCards.value;
  return [
    { label: t('task.allCards'), value: undefined },
    ...cards.map((card) => ({ label: card.uuid, value: card.uuid })),
  ];
});
const statusCounts = ref(null);
const statusTabOptions = computed(() => {
  const counts = statusCounts.value;
  return [
    { value: '', label: t('task.statusAll'), count: counts ? counts.all ?? 0 : undefined },
    ...getWorkloadStatusOptions(t).map((option) => ({
      ...option,
      count: counts ? counts[option.value] ?? 0 : undefined,
      tone: option.value === 'abnormal' ? 'danger' : undefined,
    })),
  ];
});

const fetchFilterOptions = async () => {
  try {
    const [{ list: nodeList = [] }, { list: cardList = [] }] = await Promise.all([
      request(nodeApi.getNodeList({ filters: {} })),
      request(cardApi.getCardList({ filters: {} })),
    ]);
    rawNodeNames.value = nodeList
      .map((item) => item?.name)
      .filter(Boolean);
    rawCards.value = cardList
      .filter((item) => item?.uuid)
      .map((item) => ({ uuid: item.uuid, nodeName: item.nodeName }));
  } catch {
    rawNodeNames.value = [];
    rawCards.value = [];
  }
};

const baseColumns = computed(() => [
  {
    title: t('task.workload'),
    dataIndex: 'name',
    hideTooltip: true,
    render: (workload) => {
      const { name, appName, podUid, namespace, namespaceName, request: resourceRequest, scheduling, containerKind } = workload;
      const to = buildWorkloadDetailLocation({ podUid, name });
      const workloadPodName = appName || '--';
      const workloadContainerName = name || '--';
      const workloadNamespace = namespace || namespaceName || '--';
      const workloadName = formatWorkloadName({ appName, name });
      const identityLabel = (
        <span class="workload-identity-label">
          {workloadPodName !== workloadContainerName && (
            <>
              <span class="workload-pod-name">
                <EllipsisText text={workloadPodName} mode="middle" tooltip="always" />
              </span>
              <span class="workload-identity-separator" aria-hidden="true">/</span>
            </>
          )}
          <span class="workload-container-name">
            <EllipsisText text={workloadContainerName} mode="end" tooltip="overflow" />
          </span>
        </span>
      );
      const containerKindBadge = ['init', 'sidecar'].includes(containerKind) ? (
        <span class="workload-container-kind">{t(`scheduling.containerKind.${containerKind}`)}</span>
      ) : null;
      return (
        <span style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <span class="task-name-icon-card vgpu-table-name-icon-card">
            <svg-icon icon="task-name" style={{ fontSize: '20px' }} />
          </span>
          <span class="task-name-text-wrap vgpu-table-name-text-wrap">
            <span class="workload-identity">
              {scheduling && resourceRequest ? (
                <button
                  type="button"
                  class="workload-identity-primary workload-identity-link workload-identity-button"
                  aria-label={workloadName}
                  aria-haspopup="dialog"
                  onClick={() => { selectedSchedulingContainer.value = name; selectedSchedulingPod.value = scheduling; }}
                >
                  {identityLabel}
                  {containerKindBadge}
                </button>
              ) : to ? (
                <RouterLink class="workload-identity-primary workload-identity-link" to={to} aria-label={workloadName}>
                  {identityLabel}
                  {containerKindBadge}
                </RouterLink>
              ) : (
                <span class="workload-identity-primary">
                  {identityLabel}
                  {containerKindBadge}
                </span>
              )}
              <span class="workload-namespace-line">
                <span class="workload-namespace-label">{t('task.namespace')}:</span>
                <span class="task-namespace-text">
                  <EllipsisText text={workloadNamespace} mode="end" tooltip="overflow" />
                </span>
              </span>
            </span>
          </span>
        </span>
      );
    },
  },
  {
    title: t('task.status'),
    dataIndex: 'status',
    render: (workload) => <WorkloadStatus workload={workload} />,
  },
  {
    title: t('task.resourceConfiguration'),
    dataIndex: 'deviceIds',
    render: ({ deviceIds, allocatedCores, allocatedCoresKnown, allocatedMem, request: resourceRequest }) => {
      const ids = Array.isArray(deviceIds) ? deviceIds : [];
      const totals = resourceRequest ? getWorkloadRequestTotals(resourceRequest) : {
        count: ids.length || null,
        cores: allocatedCoresKnown !== false ? allocatedCores : null,
        memoryMiB: allocatedMem,
      };
      const gpuCount = totals.count ?? '--';
      const cores = totals.cores !== null && totals.cores !== undefined
        ? roundToDecimal(totals.cores / 100, 2) : '--';
      const memoryGiB = totals.memoryMiB !== null && totals.memoryMiB !== undefined
        ? `${roundToDecimal(totals.memoryMiB / 1024, 2)} GiB` : '--';
      return (
        <div class="task-gpu-cell">
          <span class="task-gpu-cell-icon" aria-hidden="true">
            <svg-icon icon="vgpu-card" style={{ fontSize: '14px' }} />
          </span>
          <span class="task-gpu-cell-info">
            <span>{gpuCount}</span>
            <span class="task-gpu-cell-segment">{cores}</span>
            <span class="task-gpu-cell-segment">{memoryGiB}</span>
          </span>
        </div>
      );
    },
  },
  {
    title: t('task.createTime'),
    dataIndex: 'createTime',
    render: ({ createTime }) => timeParse(createTime),
  },

]);
const { eyeColumnKeys, columnOptions, visibleColumns } = useTableColumnVisibility(baseColumns);

const tableState = reactive(createRequestState([]));
const pagination = reactive({ total: 0, current: 1, pageSize: 10, pageSizeOptions: [10, 20, 50, 100], showJumper: false });

// Keep filters and page in the address so Back restores them.
const route = useRoute();
const router = useRouter();
const syncsRoute = !props.hideTitle && !props.filters;
const listPath = route.path;
const ROUTE_STATUSES = ['pending', 'waiting', 'success', 'abnormal'];
const DEFAULT_PAGE_SIZE = 10;
const firstQueryValue = (value) => (Array.isArray(value) ? value[0] : value);
const queryText = (query, key) => {
  const value = firstQueryValue(query?.[key]);
  return typeof value === 'string' ? value : '';
};
const sameQuery = (left = {}, right = {}) => [...new Set([...Object.keys(left), ...Object.keys(right)])]
  .every((key) => queryText(left, key) === queryText(right, key));
let lastRouteQuery = syncsRoute ? { ...route.query } : {};
const applyRouteQuery = (query) => {
  const status = queryText(query, 'status');
  const page = Number(queryText(query, 'page'));
  const pageSize = Number(queryText(query, 'pageSize'));
  filters.name = queryText(query, 'name');
  filters.nodeName = queryText(query, 'nodeName') || undefined;
  filters.deviceId = queryText(query, 'deviceId') || undefined;
  filters.status = ROUTE_STATUSES.includes(status) ? status : '';
  hasManualNodeScope.value = Boolean(filters.nodeName);
  pagination.current = Number.isInteger(page) && page > 0 ? page : 1;
  pagination.pageSize = pagination.pageSizeOptions.includes(pageSize) ? pageSize : DEFAULT_PAGE_SIZE;
};
const syncRouteQuery = () => {
  if (!syncsRoute || route.path !== listPath) return;
  const name = typeof filters.name === 'string' ? filters.name.trim() : '';
  const query = {
    ...(filters.status ? { status: filters.status } : {}),
    ...(filters.nodeName ? { nodeName: filters.nodeName } : {}),
    ...(filters.deviceId ? { deviceId: filters.deviceId } : {}),
    ...(name ? { name } : {}),
    ...(pagination.current > 1 ? { page: String(pagination.current) } : {}),
    ...(pagination.pageSize !== DEFAULT_PAGE_SIZE ? { pageSize: String(pagination.pageSize) } : {}),
  };
  lastRouteQuery = query;
  if (!sameQuery(query, route.query)) router.replace({ query }).catch(() => {});
};
let tableController;
const fetchTableData = async () => {
  syncRouteQuery();
  const hasResolved = tableState.hasResolved && tableState.status === REQUEST_STATUS.READY;
  const requestId = startRequest(tableState, { hasResolved });
  tableController?.abort();
  tableController = new AbortController();
  const baseFilters = { ...(props.filters || {}) };
  delete baseFilters.nodeName;
  delete baseFilters.nodeUid;
  delete baseFilters.name;
  delete baseFilters.status;
  delete baseFilters.deviceId;
  const nodeName = hasManualNodeScope.value ? filters.nodeName : props.filters?.nodeName;
  const nodeUid = hasManualNodeScope.value ? undefined : props.filters?.nodeUid;
  const payload = {
    filters: {
      ...baseFilters,
      ...(getTrimValue(filters.name) ? { name: getTrimValue(filters.name) } : {}),
      ...(nodeName ? { nodeName } : {}),
      ...(nodeUid ? { nodeUid } : {}),
      ...(filters.status ? { status: filters.status } : {}),
      ...(filters.deviceId ? { deviceId: filters.deviceId } : {}),
    },
    page: pagination.current,
    pageSize: pagination.pageSize,
  };
  try {
    const result = await taskApi.getWorkloads(payload, tableController.signal);
    if (!isLatestRequest(tableState, requestId)) return;
    const total = Number(result?.total ?? 0);
    if (!Array.isArray(result?.items) || !Number.isInteger(total) || total < 0) {
      rejectRequest(tableState, new TypeError('Expected workload items and a nonnegative total'), {
        requestId, hasResolved, status: REQUEST_STATUS.INVALID,
      });
      return;
    }
    pagination.total = total;
    // Older backends omit status counts; the filter then shows labels only.
    const counts = result?.statusCounts;
    statusCounts.value = counts && typeof counts === 'object' && Object.values(counts).every(Number.isInteger) ? counts : null;
    const lastPage = Math.max(1, Math.ceil(pagination.total / pagination.pageSize));
    if (pagination.current > lastPage) {
      pagination.current = lastPage;
      await fetchTableData();
      return;
    }
    resolveRequest(tableState, {
      requestId,
      data: result.items.map((item) => ({
        ...item,
        workloadRowKey: `${createWorkloadRowKey(item)}/${item.containerKind || 'regular'}`,
      })),
    });
  } catch (error) {
    rejectRequest(tableState, error, { requestId, hasResolved });
  }
};
const {
  data: tableData,
  refreshError: tableRefreshError,
  refreshing: tableRefreshing,
  status: tableStatus,
} = toRefs(tableState);
const resetToFirstPage = () => { pagination.current = 1; };
const changePage = ({ current, pageSize }) => {
  pagination.current = pageSize === pagination.pageSize ? current : 1;
  pagination.pageSize = pageSize;
  fetchTableData();
};
const onSchedulingUpdated = (pod) => {
  if (pod.nodeName || ['bound', 'terminating', 'finished'].includes(pod.stage)) {
    if (tableData.value.some((item) => item.pending && item.podUid === pod.uid)) fetchTableData();
    return;
  }
  tableData.value = tableData.value.map((item) => (
    item.pending && item.podUid === pod.uid ? { ...item, scheduling: pod } : item
  ));
};
const { getTrimValue, applyFilters, refreshTable } = useTableFilters({
  fetchTableData,
  resetBeforeApply: resetToFirstPage,
});
const onNodeNameChange = () => {
  hasManualNodeScope.value = true;
  // The card dropdown is scoped to the selected node; drop a previously chosen
  // card if it doesn't belong to that node so the table isn't filtered by an
  // out-of-scope device.
  if (filters.nodeName && filters.deviceId) {
    const stillValid = rawCards.value.some(
      (card) => card.nodeName === filters.nodeName && card.uuid === filters.deviceId,
    );
    if (!stillValid) {
      filters.deviceId = undefined;
    }
  }
  applyFilters();
};

onMounted(() => {
  fetchFilterOptions();
});
onBeforeUnmount(() => {
  tableState.requestId += 1;
  tableController?.abort();
});

watch(
  () => [
    props.filters?.name,
    props.filters?.nodeName,
    props.filters?.nodeUid,
    props.filters?.status,
    props.filters?.deviceId,
  ],
  () => {
    if (syncsRoute) {
      applyRouteQuery(route.query);
      fetchTableData();
      return;
    }
    hasManualNodeScope.value = false;
    filters.name = props.filters?.name || '';
    filters.nodeName = props.filters?.nodeName;
    filters.status = props.filters?.status;
    filters.deviceId = props.filters?.deviceId;
    applyFilters();
  },
  { immediate: true },
);
// Links to this page, such as the sidebar entry, reset the filters.
watch(() => route.query, (query) => {
  if (!syncsRoute || route.path !== listPath || sameQuery(query, lastRouteQuery)) return;
  applyRouteQuery(query);
  fetchTableData();
});
</script>

<style scoped lang="scss">
.task-admin-page {
  &.is-embedded {
    .task-admin-table-wrap {
      margin-top: 15px;
    }
  }
}

.task-admin-top-wrap {
  display: flex;
  flex-direction: column;

  :deep(.home-block) {
    margin-bottom: 0;
  }
}

.task-admin-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;

  :deep(.workload-table) {
    margin-top: 8px;
  }
}

:deep(.task-name-icon-card) {
  user-select: none;
}

:deep(.task-name-text-wrap) {
  flex: 1;
}

:deep(.workload-identity) {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  min-width: 0;
}

:deep(.workload-identity-primary) {
  display: flex;
  width: 100%;
  min-width: 0;
  line-height: 20px;
}

:deep(.workload-identity-link) {
  color: #324558;
  font-weight: 500;
  text-decoration: none;
}

.workload-filters {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

:deep(.workload-status-filter) {
  max-width: 100%;
  overflow-x: auto;
  scrollbar-width: none;

  &::-webkit-scrollbar {
    display: none;
  }
}

:deep(.workload-filter-select) {
  flex: 0 0 140px;
  width: 140px;
}

:deep(.workload-search) {
  flex: 1 1 170px;
  min-width: 170px;
  max-width: 280px;
}

:deep(.workload-identity-button) {
  padding: 0;
  border: 0;
  background: transparent;
  font-family: inherit;
  font-size: inherit;
  text-align: left;
  cursor: pointer;
}

:deep(.workload-identity-link:hover),
:deep(.workload-identity-link:focus-visible) {
  color: var(--el-color-primary);
}

:deep(.workload-identity-label) {
  position: relative;
  display: inline-flex;
  flex: 0 1 auto;
  align-items: baseline;
  gap: 6px;
  max-width: 100%;
  min-width: 0;
  line-height: inherit;
}

:deep(.workload-pod-name) {
  display: flex;
  flex: 0 1 auto;
  min-width: 0;
  max-width: 240px;
  overflow: hidden;
  line-height: inherit;
}

:deep(.workload-identity-separator) {
  flex: 0 0 auto;
  color: inherit;
  line-height: inherit;
}

:deep(.workload-container-name) {
  display: flex;
  flex: 0 0 auto;
  min-width: 0;
  max-width: 240px;
  overflow: hidden;
  line-height: inherit;
}

:deep(.workload-container-kind) {
  flex: 0 0 auto;
  align-self: center;
  padding: 0 4px;
  margin-left: 6px;
  border-radius: 3px;
  background: #e4ebf1;
  color: #697886;
  font-size: 11px;
  font-weight: 400;
  line-height: 18px;
  white-space: nowrap;
}

:deep(.workload-identity-label::after) {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 1px;
  background: currentcolor;
  content: '';
  opacity: 0;
  pointer-events: none;
}

:deep(.workload-identity-link:hover .workload-identity-label::after),
:deep(.workload-identity-link:focus-visible .workload-identity-label::after) {
  opacity: 1;
}

:deep(.workload-namespace-line) {
  display: flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
  font-size: 12px;
  line-height: 16px;
}

:deep(.workload-namespace-label) {
  flex: 0 0 auto;
  color: #939ea9;
  line-height: inherit;
  white-space: nowrap;
}

:deep(.task-namespace-text) {
  display: flex;
  align-items: baseline;
  min-width: 0;
  overflow: hidden;
  color: #939ea9;
  line-height: inherit;
}

:deep(.task-gpu-cell) {
  display: inline-flex;
  width: fit-content;
  align-items: center;
  gap: 8px;
  padding: 4px 12px 4px 4px;
  border-radius: 6px;
  background: #e4ebf1;
  color: #324558;
  font-size: 14px;
  font-weight: 500;
}

:deep(.task-gpu-cell-icon) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 4px;
  background: #fff;
  box-shadow:
    0 1px 1px 0 rgb(2 5 8 / 2%),
    0 1px 4px 0 rgb(2 5 8 / 6%);
  flex-shrink: 0;
}

:deep(.task-gpu-cell-info) {
  display: inline-flex;
  align-items: center;
}

:deep(.task-gpu-cell-segment) {
  margin-left: 8px;
  padding-left: 8px;
  border-left: 1px solid #d5dee7;
}

</style>
