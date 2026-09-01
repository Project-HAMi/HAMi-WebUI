<template>
  <div class="task-admin-page vgpu-admin-page" :class="{ 'is-embedded': hideTitle }">
    <div v-if="!hideTitle" class="vgpu-admin-page-title">{{ $t('task.title') }}</div>

    <div class="task-admin-top-wrap" v-if="!hideTitle">
      <Top />
    </div>

    <div class="task-admin-table-wrap">
      <toolbar
        v-model="eyeColumnKeys"
        :column-options="columnOptions"
        @refresh="refreshTable"
      >
        <t-space :size="8">
          <t-select
            v-model="filters.nodeName"
            clearable
            :placeholder="$t('task.allNodes')"
            :options="nodeOptions"
            @change="onNodeNameChange"
          />
          <t-select
            v-model="filters.status"
            clearable
            :placeholder="$t('task.allStatus')"
            :options="statusOptions"
            @change="applyFilters"
          />
          <t-select
            v-model="filters.deviceId"
            clearable
            :placeholder="$t('task.allCards')"
            :options="cardOptions"
            @change="applyFilters"
          />
          <t-input
            v-model="filters.name"
            clearable
            :placeholder="$t('task.searchWorkloadName')"
            @enter="applyFilters"
            @blur="applyFilters"
          >
            <template #prefix-icon>
              <search-icon :style="{ cursor: 'pointer' }" />
            </template>
          </t-input>
        </t-space>
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
          :data="pagedTableData"
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
            @update:current="(val) => (pagination.current = val)"
            @update:pageSize="(val) => (pagination.pageSize = val)"
          />
        </template>
      </stateful-table>
    </div>

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
import { SearchIcon, HelpCircleIcon } from 'tdesign-icons-vue-next';
import { reactive, ref, computed, onMounted, watch } from 'vue';
import { RouterLink } from 'vue-router';
import Top from './top.vue';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import useTableFilters from '~/vgpu/hooks/useTableFilters';
import useLocalPagination from '~/vgpu/hooks/useLocalPagination';
import { createWorkloadRowKey, formatWorkloadName } from './workload-identity.mjs';
import useFetchList from '@/hooks/useFetchList';

const props = defineProps(['hideTitle', 'filters', 'style']);
const { t, locale } = useI18n();
const hasManualNodeScope = ref(false);
const filters = reactive({
  name: props.filters?.name || '',
  nodeName: props.filters?.nodeName,
  status: props.filters?.status,
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
const statusOptions = computed(() => [
  { label: t('task.allStatus'), value: undefined },
  { label: t('task.statusCompleted'), value: 'closed' },
  { label: t('task.statusRunning'), value: 'success' },
  { label: t('task.statusFailed'), value: 'failed' },
  { label: t('task.statusUnknown'), value: 'unknown' },
]);

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
    render: ({ name, appName, podUid, namespace, namespaceName }) => {
      const to = `/admin/vgpu/task/admin/detail?name=${name}&podUid=${podUid}`;
      const workloadPodName = appName || '--';
      const workloadContainerName = name || '--';
      const workloadNamespace = namespace || namespaceName || '--';
      const workloadName = formatWorkloadName({ appName, name });
      return (
        <span style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <span class="task-name-icon-card vgpu-table-name-icon-card">
            <svg-icon icon="task-name" style={{ fontSize: '20px' }} />
          </span>
          <span class="task-name-text-wrap vgpu-table-name-text-wrap">
            <span class="workload-identity">
              <RouterLink
                class="workload-identity-primary workload-identity-link"
                to={to}
                aria-label={workloadName}
              >
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
              </RouterLink>
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
    render: ({ status }) => {
      const enums = {
        closed: {
          text: t('task.statusCompleted'),
          icon: 'status-schedulable',
        },
        success: {
          text: t('task.statusRunning'),
          icon: 'status-schedulable',
        },
        unknown: {
          text: t('task.statusUnknown'),
          icon: 'status-unmanaged',
        },
        failed: {
          text: t('task.statusFailed'),
          icon: 'status-unschedulable',
        },
      };
      const { text, icon } = enums[status] || enums.unknown;
      return (
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '6px',
          }}
        >
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
          {(status === 'unknown' || status === 'failed') && (
            <t-popup
              trigger="hover"
              placement="top"
              content={t('task.checkCloudPlatform')}
              overlay-inner-style={{ maxWidth: '180px' }}
            >
              <help-circle-icon style={{ color: '#939EA9', fontSize: '14px', cursor: 'pointer' }} />
            </t-popup>
          )}
        </span>
      );
    },
  },
  {
    title: t('task.card'),
    dataIndex: 'deviceIds',
    render: ({ deviceIds, allocatedCores, allocatedCoresKnown, allocatedMem }) => {
      const ids = Array.isArray(deviceIds) ? deviceIds : [];
      const gpuCount = ids.length || '--';
      const cores = allocatedCoresKnown !== false && (allocatedCores === 0 || allocatedCores)
        ? roundToDecimal(allocatedCores / 100, 1)
        : '--';
      const memoryGiB = allocatedMem === 0 || allocatedMem
        ? `${roundToDecimal(allocatedMem / 1024, 1)} GiB`
        : '--';
      return (
        <div class="task-gpu-cell">
          <span class="task-gpu-cell-icon" aria-hidden="true">
            <svg-icon icon="card-id" style={{ fontSize: '14px' }} />
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

const tableState = useFetchList(() => {
  const baseFilters = { ...(props.filters || {}) };
  delete baseFilters.nodeName;
  delete baseFilters.nodeUid;
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
  };
  return taskApi.getTaskListReq(payload);
}, {
  immediate: false,
  path: 'items',
  mapData: (items) => items.map((item) => ({
    ...item,
    workloadRowKey: createWorkloadRowKey(item),
  })),
});
const {
  data: tableData,
  refresh: fetchTableData,
  refreshError: tableRefreshError,
  refreshing: tableRefreshing,
  status: tableStatus,
} = tableState;
const { pagination, pagedTableData, syncTotalAndClamp, resetToFirstPage } = useLocalPagination(tableData);
watch(tableData, syncTotalAndClamp, { immediate: true, flush: 'sync' });
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

watch(
  () => [
    props.filters?.name,
    props.filters?.nodeName,
    props.filters?.nodeUid,
    props.filters?.status,
    props.filters?.deviceId,
  ],
  () => {
    hasManualNodeScope.value = false;
    filters.name = props.filters?.name || '';
    filters.nodeName = props.filters?.nodeName;
    filters.status = props.filters?.status;
    filters.deviceId = props.filters?.deviceId;
    applyFilters();
  },
  { immediate: true },
);
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

  :deep(.workload-table .t-table__body td) {
    padding-top: 4px;
    padding-bottom: 4px;
  }

  :deep(.workload-table .t-table__body td:first-child) {
    line-height: 0;
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
  gap: 2px;
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
