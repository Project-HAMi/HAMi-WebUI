<template>
  <div class="node-admin-page vgpu-admin-page">
    <div class="vgpu-admin-page-title">{{ $t('node.title') }}</div>

    <div class="node-admin-top-wrap">
      <preview-bar :handle-click="handleClick" />
    </div>

    <div class="node-admin-table-wrap">
      <toolbar
        v-model="eyeColumnKeys"
        :column-options="columnOptions"
        :refreshing="tableRefreshing"
        @refresh="refreshTable"
      >
        <t-space :size="8">
          <t-select
            v-model="filters.schedulingEligibility"
            clearable
            :popup-props="{ overlayClassName: 'select-option-label-popup', overlayInnerStyle: fitPopupToOptions }"
            :placeholder="$t('node.allSchedulingEligibilityStatuses')"
            :options="statusOptions"
          />
          <t-select
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
            class="list-keyword-search"
            clearable
            :placeholder="$t('node.searchKeyword')"
            :aria-label="$t('node.searchKeyword')"
          >
            <template #prefix-icon>
              <search-icon aria-hidden="true" />
            </template>
          </t-input>
        </t-space>
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
          :data="visibleNodes"
          :columns="visibleColumns"
          table-layout="auto"
        />
      </stateful-table>
    </div>
  </div>
</template>

<script setup lang="jsx">
import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';
import { useRouter, useRoute } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import StatefulTable from '@/components/TablePlus/StatefulTable.vue';
import TextPlus from '@/components/TextPlus.vue';
import SelectOptionLabel from '@/components/SelectOptionLabel.vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import { MessagePlugin } from 'tdesign-vue-next';
import request from '@/utils/request';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { computed, ref, reactive, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import { readQuery, sameQuery, writeQuery } from '~/vgpu/hooks/route-filters.mjs';
import { createOptionFilter, matchesKeywords, parseKeywords } from '@/utils/keyword-search.mjs';
import { fitPopupToOptions } from '@/utils/select-popup.mjs';
import useFetchList from '@/hooks/useFetchList';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import {
  getNodeSchedulingEligibilityStatus,
  matchesNodeSchedulingEligibility,
} from '~/vgpu/views/node/node-status.mjs';

const router = useRouter();
const route = useRoute();
const { t, locale } = useI18n();
const allNodeMap = ref(new Map());
const filters = reactive({
  schedulingEligibility: undefined,
  type: undefined,
  keyword: '',
});
const filterOption = createOptionFilter();
const rawCardTypes = ref([]);
const statusOption = (key, value) => ({
  label: t(key),
  value,
  content: () => <SelectOptionLabel label={t(key)} {...nodeCount(statusNodeCount.value(value))} />,
});
const statusOptions = computed(() => [
  { label: t('node.allSchedulingEligibilityStatuses'), value: undefined },
  statusOption('node.schedulable', 'schedulable'),
  statusOption('node.temporarilyUnschedulable', 'temporarilyUnschedulable'),
]);
const cardTypeOptions = computed(() => [
  // An empty value keeps "All" pickable without showing its label as typed text before a filter is chosen.
  { label: t('node.allTypes'), value: '' },
  ...rawCardTypes.value.map((type) => ({
    label: type,
    value: type,
    content: () => <SelectOptionLabel label={type} {...nodeCount(typeNodeCounts.value.get(type))} />,
  })),
]);

const fetchCardTypeOptions = async () => {
  try {
    const { list = [] } = await request(cardApi.getCardType());
    rawCardTypes.value = list
      .map((item) => item?.type)
      .filter(Boolean);
  } catch {
    rawCardTypes.value = [];
  }
};

const handleClick = (params) => {
  const name = params.data.name;
  const uid = allNodeMap.value.get(name);
  if (uid) {
    const uuid = uid;
    router.push(`/nodes/${uuid}?nodeName=${name}`);
  } else {
    MessagePlugin.error(t('node.nodeNotFound'));
  }
};

const fetchAllNodeMap = async () => {
  try {
    const { list = [] } = await request(nodeApi.getNodeList({ filters: {} }));
    const nextMap = new Map();
    list.forEach((item) => {
      if (item?.name && item?.uid) {
        nextMap.set(item.name, item.uid);
      }
    });
    allNodeMap.value = nextMap;
  } catch {
    allNodeMap.value = new Map();
  }
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

const baseColumns = computed(() => [
  {
    title: t('node.name'),
    minWidth: 200,
    dataIndex: 'name',
    render: ({ uid, name }) => {
      const to = `/nodes/${uid}?nodeName=${name}`;
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
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
    render: ({ coreTotal, coreUsed, coreUsedKnown, isExternal }) => {
      if (isExternal || coreUsedKnown === false || !coreTotal) return <span>--</span>;
      const rawPercent = (Number(coreUsed || 0) / Number(coreTotal)) * 100;
      const percentForProgress = Math.max(0, Math.min(100, rawPercent));
      const color = getResourceColor(percentForProgress);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
          <t-progress
            class="node-admin-circle-progress"
            theme="circle"
            size={24}
            strokeWidth={3}
            percentage={roundToDecimal(percentForProgress, 2)}
            color={color}
            label={false}
          />
          <span>{roundToDecimal(rawPercent, 2)}%</span>
        </span>
      );
    },
  },
  {
    title: t('node.memoryAllocTotal'),
    key: 'node-memory-allocation',
    minWidth: 280,
    dataIndex: 'used',
    render: ({ memoryTotal, memoryUsed, isExternal }) => {
      if (isExternal || !memoryTotal) return <span>--</span>;
      const rawPercent = (Number(memoryUsed || 0) / Number(memoryTotal)) * 100;
      const percentForProgress = Math.max(0, Math.min(100, rawPercent));
      const color = getResourceColor(percentForProgress);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
          <t-progress
            class="node-admin-circle-progress"
            theme="circle"
            size={24}
            strokeWidth={3}
            percentage={roundToDecimal(percentForProgress, 2)}
            color={color}
            label={false}
          />
          <span>{roundToDecimal(rawPercent, 2)}%</span>
        </span>
      );
    },
  },
]);
const { eyeColumnKeys, columnOptions, visibleColumns } = useTableColumnVisibility(baseColumns);

// The list is small enough to load once; every filter runs locally while typing.
const tableState = useFetchList(() => request(nodeApi.getNodeList({ filters: {} })), { immediate: false });
const {
  data: tableData,
  refresh: refreshTable,
  refreshError: tableRefreshError,
  refreshing: tableRefreshing,
  status: tableStatus,
} = tableState;
const keywords = computed(() => parseKeywords(filters.keyword));
const filtered = computed(() => Boolean(filters.type || filters.schedulingEligibility || keywords.value.length));
const keywordNodes = computed(() => tableData.value.filter((node) =>
  matchesKeywords(keywords.value, [node.name, node.ip, node.uid, ...(node.type ?? [])])));
const hasType = (node, type) => !type || (node.type ?? []).includes(type);
const visibleNodes = computed(() => keywordNodes.value.filter((node) =>
  matchesNodeSchedulingEligibility(node, filters.schedulingEligibility) && hasType(node, filters.type)));
// Options show how many nodes picking them would list under the other filters.
const statusNodeCount = computed(() => {
  const nodes = keywordNodes.value.filter((node) => hasType(node, filters.type));
  return (status) => nodes.filter((node) => matchesNodeSchedulingEligibility(node, status)).length;
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
  filters.schedulingEligibility = undefined;
};

const parseSchedulingEligibility = (value) =>
  (value === 'schedulable' || value === 'temporarilyUnschedulable' ? value : undefined);
const ROUTE_KEYS = ['q', 'schedulingEligibility', 'type'];
watch(
  () => route.query,
  (query) => {
    const values = readQuery(query, ROUTE_KEYS);
    // Keep the typed text, including spaces between words, when the query only trimmed it.
    if ((values.q ?? '') !== filters.keyword.trim()) filters.keyword = values.q ?? '';
    filters.schedulingEligibility = parseSchedulingEligibility(values.schedulingEligibility);
    filters.type = values.type;
  },
  { immediate: true },
);
watch(
  () => [filters.keyword, filters.schedulingEligibility, filters.type],
  () => {
    const query = writeQuery(route.query, {
      q: filters.keyword.trim(),
      schedulingEligibility: filters.schedulingEligibility,
      type: filters.type,
    });
    if (!sameQuery(query, route.query)) router.replace({ query });
  },
);

onMounted(() => {
  refreshTable();
  fetchCardTypeOptions();
  fetchAllNodeMap();
});
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
