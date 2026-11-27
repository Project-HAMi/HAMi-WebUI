<template>
  <div class="node-admin-page vgpu-admin-page">
    <div class="vgpu-admin-page-title">{{ $t('node.title') }}</div>

    <div class="node-admin-top-wrap">
      <preview-bar
        :handle-click="handleClick"
        :model-distribution="vendorDistribution"
        :rankings="rankings"
        :ranking-status="tableStatus"
      />
    </div>

    <div class="node-admin-table-wrap">
      <toolbar
        v-model="eyeColumnKeys"
        :column-options="columnOptions"
        :refreshing="tableRefreshing"
        @refresh="refreshTable"
      >
        <div class="inventory-list-filters">
          <SegmentedControl
            v-model="filters.schedulingEligibility"
            :options="statusOptions"
            :aria-label="$t('dashboard.nodeStatus')"
          />
          <t-select
            class="inventory-filter-select"
            v-model="filters.type"
            clearable
            filterable
            :filter="filterOption"
            :popup-props="{ overlayClassName: 'select-option-label-popup', overlayInnerStyle: fitPopupToOptions }"
            :placeholder="$t('node.allTypes')"
            :options="cardTypeOptions"
          />
          <t-input
            v-model="filters.keyword"
            class="list-keyword-search inventory-filter-search"
            clearable
            :placeholder="$t('node.searchKeyword')"
            :aria-label="$t('node.searchKeyword')"
          >
            <template #prefix-icon>
              <search-icon aria-hidden="true" />
            </template>
          </t-input>
        </div>
      </toolbar>
      <stateful-table
        :status="tableStatus"
        :refreshing="tableRefreshing"
        :refresh-error="tableRefreshError"
        :has-rows="visibleNodes.length > 0"
        :filtered="filtered"
        :column-count="visibleColumns.length"
        :columns="visibleColumns"
        table-layout="auto"
        @retry="refreshTable"
        @clear-filters="clearFilters"
      >
        <t-table
          :key="locale"
          row-key="uid"
          class="node-table vgpu-table-skin"
          :data="pagedNodes"
          :columns="visibleColumns"
          table-layout="auto"
        />
        <template #footer>
          <table-pagination
            :total="pagination.total"
            :current="pagination.current"
            :page-size="pagination.pageSize"
            :page-sizes="pagination.pageSizeOptions"
            @change="handlePageChange"
          />
        </template>
      </stateful-table>
    </div>
  </div>
</template>

<script setup lang="jsx">
import apiClient from '@/api/client.mjs';
import { useRouter, useRoute } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import SegmentedControl from '@/components/SegmentedControl/index.vue';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import StatefulTable from '@/components/TablePlus/StatefulTable.vue';
import TablePagination from '@/components/TablePlus/Pagination.vue';
import TextPlus from '@/components/TextPlus.vue';
import { buildNodeDetailLocation } from './detail-location.mjs';
import SelectOptionLabel from '@/components/SelectOptionLabel.vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import { MessagePlugin } from 'tdesign-vue-next';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { computed, reactive, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import useLocalPagination from '~/vgpu/hooks/useLocalPagination';
import { readQuery, sameQuery, writeQuery } from '~/vgpu/hooks/route-filters.mjs';
import { createOptionFilter, matchesKeywords, parseKeywords } from '@/utils/keyword-search.mjs';
import { fitPopupToOptions } from '@/utils/select-popup.mjs';
import useFetchList from '@/hooks/useFetchList';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import {
  getNodeSchedulingEligibilityStatus,
  matchesNodeSchedulingEligibility,
} from '~/vgpu/views/nodes/node-status.mjs';
import { buildNodeRankings, buildNodeVendorDistribution } from './node-rankings.mjs';

const router = useRouter();
const route = useRoute();
const { t, locale } = useI18n();
const filters = reactive({
  schedulingEligibility: '',
  type: undefined,
  keyword: '',
});
// Filter the full result before paginating so option counts cover every match.
const tableState = useFetchList(async () => {
  const reply = await apiClient.listNodes();
  if (reply?.inventoryCoverage?.entity !== 'node') {
    throw new Error('Node inventory coverage is missing');
  }
  return reply;
}, { immediate: false });
const {
  data: tableData,
  refresh: refreshTable,
  refreshError: tableRefreshError,
  refreshing: tableRefreshing,
  status: tableStatus,
} = tableState;
const filterOption = createOptionFilter();
const vendorDistribution = computed(() => buildNodeVendorDistribution(tableData.value));
const rankings = computed(() => buildNodeRankings(tableData.value));
const nodeModels = computed(() => [...new Set(tableData.value.flatMap((node) => node.type ?? []))]
  .filter(Boolean).sort());
const statusOptions = computed(() => [
  { value: '', label: t('task.statusAll'), count: statusNodeCount.value('') },
  { value: 'schedulable', label: t('node.schedulable'), count: statusNodeCount.value('schedulable') },
  { value: 'temporarilyUnschedulable', label: t('node.temporarilyUnschedulable'), count: statusNodeCount.value('temporarilyUnschedulable') },
]);
const cardTypeOptions = computed(() => [
  // An empty value keeps "All" pickable without showing its label as typed text before a filter is chosen.
  { label: t('node.allTypes'), value: '' },
  ...nodeModels.value.map((type) => ({
    label: type,
    value: type,
    content: () => <SelectOptionLabel label={type} {...nodeCount(typeNodeCounts.value.get(type))} />,
  })),
]);

const handleClick = ({ data: { uid, name } = {} }) => {
  const node = tableData.value.find((row) => (uid && row.uid === uid) || row.name === name);
  const to = buildNodeDetailLocation({ uid: node?.uid, nodeName: node?.name || name });
  if (to) router.push(to);
  else MessagePlugin.error(t('node.nodeNotFound'));
};

const renderNodeStatus = (row) => {
  const { icon, labelKey, descriptionKey, showHelp } =
    getNodeSchedulingEligibilityStatus(row);
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
      <svg-icon icon={icon} style={{ fontSize: '16px' }} aria-hidden="true" />
      <span>{t(labelKey)}</span>
      {showHelp && (
        <MetricHelp
          description={t(descriptionKey)}
          helpLabel={t('node.schedulingStatusHelpLabel')}
        />
      )}
    </span>
  );
};

const renderAllocation = (percent) => {
  if (!Number.isFinite(percent) || percent < 0) return <span>--</span>;
  const progress = Math.min(100, percent);
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
      <t-progress
        class="node-admin-circle-progress"
        theme="circle"
        size={24}
        strokeWidth={3}
        percentage={roundToDecimal(progress, 2)}
        color={getResourceColor(progress)}
        label={false}
      />
      <span>{roundToDecimal(percent, 2)}%</span>
    </span>
  );
};

const baseColumns = computed(() => [
  {
    title: t('node.name'),
    minWidth: 200,
    dataIndex: 'name',
    render: ({ uid, name }) => {
      const to = buildNodeDetailLocation({ uid, nodeName: name });
      return (
        <span class="vgpu-table-name-cell">
          <span class="vgpu-table-name-icon-card node-name-icon-card">
            <svg-icon icon="node-name" style={{ fontSize: '20px' }} />
          </span>
          <span class="vgpu-table-name-text-wrap">
            <TextPlus text={name} to={to} />
          </span>
        </span>
      );
    },
  },
  {
    title: t('node.status'),
    minWidth: 200,
    dataIndex: 'schedulingEligibility',
    render: (row) => renderNodeStatus(row),
  },
  {
    title: t('node.ip'),
    minWidth: 100,
    dataIndex: 'ip',
  },
  {
    title: t('node.computeAllocTotal'),
    key: 'node-compute-allocation',
    minWidth: 280,
    dataIndex: 'used',
    render: ({ coreAllocationPercent }) => renderAllocation(coreAllocationPercent),
  },
  {
    title: t('node.memoryAllocTotal'),
    key: 'node-memory-allocation',
    minWidth: 280,
    dataIndex: 'used',
    render: ({ memoryAllocationPercent }) => renderAllocation(memoryAllocationPercent),
  },
]);
const { eyeColumnKeys, columnOptions, visibleColumns } = useTableColumnVisibility(baseColumns);

const keywords = computed(() => parseKeywords(filters.keyword));
const filtered = computed(() => Boolean(filters.type || filters.schedulingEligibility || keywords.value.length));
const keywordNodes = computed(() => tableData.value.filter((node) =>
  matchesKeywords(keywords.value, [node.name, node.ip, node.uid, ...(node.type ?? [])])));
const hasType = (node, type) => !type || (node.type ?? []).includes(type);
const visibleNodes = computed(() => keywordNodes.value.filter((node) =>
  matchesNodeSchedulingEligibility(node, filters.schedulingEligibility) && hasType(node, filters.type)));
const DEFAULT_PAGE_SIZE = 10;
const { pagination, pagedTableData: pagedNodes, syncTotalAndClamp, resetToFirstPage } = useLocalPagination(visibleNodes, {
  pageSize: DEFAULT_PAGE_SIZE,
  pageSizeOptions: [10, 20, 50, 100],
});
// A link can restore a later page before its node list has arrived.
watch([() => visibleNodes.value.length, tableStatus, () => pagination.current, () => pagination.pageSize], () => {
  if (tableStatus.value === REQUEST_STATUS.READY) syncTotalAndClamp();
}, { immediate: true });
const handlePageChange = ({ current, pageSize }) => {
  pagination.current = pageSize === pagination.pageSize ? current : 1;
  pagination.pageSize = pageSize;
};
// Options show how many nodes picking them would list under the other filters.
const statusNodeCount = computed(() => {
  const nodes = keywordNodes.value.filter((node) => hasType(node, filters.type));
  return (status) => tableStatus.value === REQUEST_STATUS.READY
    ? nodes.filter((node) => matchesNodeSchedulingEligibility(node, status)).length : undefined;
});
const typeNodeCounts = computed(() => {
  const counts = new Map();
  for (const node of keywordNodes.value) {
    if (!matchesNodeSchedulingEligibility(node, filters.schedulingEligibility)) continue;
    for (const type of new Set(node.type ?? [])) counts.set(type, (counts.get(type) ?? 0) + 1);
  }
  return counts;
});
// Counts stay hidden until the list loads; a loaded empty list still shows 0.
const nodeCount = (count = 0) => (tableStatus.value === REQUEST_STATUS.READY ? { count, countLabel: t('node.nodeCount', { count }, count) } : {});
const clearFilters = () => {
  filters.keyword = '';
  filters.type = undefined;
  filters.schedulingEligibility = '';
};

const parseSchedulingEligibility = (value) =>
  (value === 'schedulable' || value === 'temporarilyUnschedulable' ? value : '');
const ROUTE_KEYS = ['q', 'schedulingEligibility', 'type', 'page', 'pageSize'];
// Reset on meaningful edits. When a link supplies both filters and a page, restore the page last.
watch([() => keywords.value.join(' '), () => filters.schedulingEligibility || '', () => filters.type || ''],
  resetToFirstPage, { flush: 'sync' });
watch(
  () => route.query,
  (query) => {
    const values = readQuery(query, ROUTE_KEYS);
    // Keep the typed text, including spaces between words, when the query only trimmed it.
    if ((values.q ?? '') !== filters.keyword.trim()) filters.keyword = values.q ?? '';
    filters.schedulingEligibility = parseSchedulingEligibility(values.schedulingEligibility);
    filters.type = values.type;
    const page = Number(values.page);
    const size = Number(values.pageSize);
    pagination.pageSize = pagination.pageSizeOptions.includes(size) ? size : DEFAULT_PAGE_SIZE;
    pagination.current = Number.isSafeInteger(page) && page > 0 ? page : 1;
  },
  { immediate: true },
);
watch(
  () => [filters.keyword, filters.schedulingEligibility, filters.type, pagination.current, pagination.pageSize],
  () => {
    const query = writeQuery(route.query, {
      q: filters.keyword.trim(),
      schedulingEligibility: filters.schedulingEligibility || undefined,
      type: filters.type,
      page: pagination.current > 1 ? pagination.current : undefined,
      pageSize: pagination.pageSize !== DEFAULT_PAGE_SIZE ? pagination.pageSize : undefined,
    });
    if (!sameQuery(query, route.query)) router.replace({ query });
  },
  { immediate: true },
);

onMounted(refreshTable);
</script>

<style scoped lang="scss">
.node-admin-page {
  :deep(.preview) {
    margin-bottom: 0;
  }
}

.node-admin-top-wrap {
  display: flex;
  flex-direction: column;
  overflow: visible;
}

.node-admin-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;

  :deep(.node-table) {
    margin-top: 8px;
  }
}

:deep(.node-name-icon-card) {
  user-select: none;
}

:deep(.vgpu-table-name-text-wrap .text-plus) {
  align-items: center;
}

:deep(.vgpu-table-name-text-wrap) {
  flex: 1;
}
</style>
