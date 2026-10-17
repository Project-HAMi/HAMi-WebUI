<template>
  <div class="card-admin-page vgpu-admin-page" :class="{ 'is-embedded': hideTitle }">
    <div v-if="!hideTitle" class="vgpu-admin-page-title">{{ $t('card.title') }}</div>

    <DeviceConfigAlert />

    <div class="card-admin-top-wrap" v-if="!hideTitle">
      <preview-bar
        :title="$t('dashboard.card')"
        type="device_uuid"
        :handle-click="handleClick"
        :handle-pie-click="handlePieClick"
        :currentName="currentType"
      />
    </div>

    <div class="card-admin-table-wrap">
      <toolbar
        v-model="eyeColumnKeys"
        :column-options="columnOptions"
        :refreshing="tableRefreshing"
        @refresh="refreshTable"
      >
        <t-space :size="8">
          <t-select
            v-model="filters.type"
            clearable
            filterable
            :filter="filterOption"
            :popup-props="{ overlayClassName: 'select-option-label-popup', overlayInnerStyle: fitPopupToOptions }"
            :placeholder="$t('card.allCardTypes')"
            :options="cardTypeOptions"
            @change="onTypeChange"
          />
          <t-select
            v-model="filters.nodeName"
            clearable
            filterable
            :popup-props="{ overlayClassName: 'select-option-label-popup', overlayInnerStyle: nodePopupStyle }"
            :panel-bottom-content="shownNodeOptions.hidden ? renderMoreOptions(shownNodeOptions.hidden) : undefined"
            :placeholder="$t('card.allNodes')"
            :options="shownNodeOptions.options"
            @search="showGivenOptions"
            @input-change="(text) => (nodeSearch = text)"
            @change="onNodeNameChange"
          />
          <t-input
            v-model="filters.keyword"
            class="list-keyword-search"
            clearable
            :placeholder="$t('card.searchKeyword')"
            :aria-label="$t('card.searchKeyword')"
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
        :has-rows="visibleCards.length > 0"
        :filtered="filtered"
        :column-count="visibleColumns.length"
        :columns="visibleColumns"
        table-layout="fixed"
        @retry="refreshTable"
        @clear-filters="clearFilters"
      >
        <t-table
          :key="`${locale}-card-table`"
          row-key="uuid"
          class="accelerator-table vgpu-table-skin"
          :data="pagedTableData"
          :columns="visibleColumns"
          table-layout="fixed"
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
import cardApi from '~/vgpu/api/card';
import nodeApi from '~/vgpu/api/node';
import { useRouter, useRoute } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import TablePagination from '@/components/TablePlus/Pagination.vue';
import StatefulTable from '@/components/TablePlus/StatefulTable.vue';
import EllipsisText from '@/components/EllipsisText.vue';
import TextPlus from '@/components/TextPlus.vue';
import request from '@/utils/request';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { ref, watch, computed, reactive, onMounted } from 'vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import { readQuery, sameQuery, writeQuery } from '~/vgpu/hooks/route-filters.mjs';
import { createOptionFilter, limitOptions, matchesKeywords, parseKeywords } from '@/utils/keyword-search.mjs';
import { fitPopupToOptions } from '@/utils/select-popup.mjs';
import SelectOptionLabel from '@/components/SelectOptionLabel.vue';
import useLocalPagination from '~/vgpu/hooks/useLocalPagination';
import { getAllocationPercent } from './allocation-percent.mjs';
import UnconfiguredTag from './components/UnconfiguredTag.vue';
import DeviceConfigAlert from '~/vgpu/components/DeviceConfigAlert.vue';
import { deviceWording } from '~/vgpu/components/device-copy.mjs';
import { getSplitIcon, getSplitModeKey } from '~/vgpu/components/split-mode.mjs';
import useFetchList from '@/hooks/useFetchList';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';

const props = defineProps(['hideTitle', 'filters']);

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const parseTypeFromQuery = (value) => {
  if (typeof value === 'string') return value || undefined;
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0] || undefined;
  return undefined;
};

const currentType = computed(() => filters.type || '');
const hasManualNodeScope = ref(false);
const filters = reactive({
  keyword: '',
  nodeName: props.filters?.nodeName,
  type: props.filters?.type ?? parseTypeFromQuery(route.query.type),
});
const rawNodes = ref([]);
const nodesLoaded = ref(false);
const rawCardTypes = ref([]);
const nodeOptions = computed(() => {
  // When a card type is selected, only offer nodes that actually carry that type.
  const nodes = filters.type
    ? rawNodes.value.filter((node) => node.types.includes(filters.type))
    : rawNodes.value;
  return [
  // An empty value keeps "All" pickable without showing its label as typed text before a filter is chosen.
    { label: t('card.allNodes'), value: '' },
    ...nodes.map((node) => ({
      label: node.name,
      value: node.name,
      content: () => <SelectOptionLabel label={node.name} hints={[node.ip]} {...cardCount(nodeCardCounts.value.get(node.name))} />,
    })),
  ];
});
const filterOption = createOptionFilter();
const nodeIps = computed(() => new Map(rawNodes.value.map((node) => [node.name, node.ip])));
// A fixed width keeps the popup steady while typing changes the matches.
const nodePopupStyle = { width: '260px', maxWidth: 'calc(100vw - 24px)' };
// Large clusters have thousands of candidates; render the first matches and filter the rest by typing.
const nodeSearch = ref('');
// An onSearch handler makes TDesign show these options as given. The text comes from
// input-change instead, which is not debounced and is cleared when the popup closes.
const showGivenOptions = () => {};
const shownNodeOptions = computed(() => limitOptions(nodeOptions.value, nodeSearch.value, (name) => [nodeIps.value.get(name)]));
const renderMoreOptions = (count) => () => <div class="select-more-hint">{t('common.moreOptionsHidden', { count })}</div>;
const cardTypeOptions = computed(() => [
  // An empty value keeps "All" pickable without showing its label as typed text before a filter is chosen.
  { label: t('card.allCardTypes'), value: '' },
  ...rawCardTypes.value.map((type) => ({
    label: type,
    value: type,
    content: () => <SelectOptionLabel label={type} {...cardCount(typeCardCount.value(type))} />,
  })),
]);

const fetchFilterOptions = async () => {
  try {
    const [{ list: nodeList = [] }, { list: typeList = [] }] = await Promise.all([
      request(nodeApi.getNodeList({ filters: {} })),
      request(cardApi.getCardType()),
    ]);
    rawNodes.value = nodeList
      .filter((item) => item?.name)
      .map((item) => ({ name: item.name, ip: item.ip, types: Array.isArray(item.type) ? item.type : [] }));
    nodesLoaded.value = true;
    rawCardTypes.value = typeList
      .map((item) => item?.type)
      .filter(Boolean);
  } catch {
    rawNodes.value = [];
    rawCardTypes.value = [];
  }
};

// When the selected type no longer includes the chosen node, drop the node so
// the table isn't filtered by an out-of-scope node. Call synchronously before
// applyFilters at every type-mutation site to avoid querying with a stale node.
const pruneNodeScopeByType = () => {
  // Unloaded or failed options must not drop a node restored from the URL.
  if (!filters.type || !filters.nodeName || !nodesLoaded.value) {
    return;
  }
  const node = rawNodes.value.find((item) => item.name === filters.nodeName);
  if (!node || !node.types.includes(filters.type)) {
    filters.nodeName = undefined;
    hasManualNodeScope.value = true;
  }
};

const onTypeChange = () => {
  pruneNodeScopeByType();
  applyFilters();
};

const handleClick = (params) => {
  router.push({
    path: `/accelerators/${params.data.name}`,
  });
};

const getCardStatusDisplay = ({ health, isExternal }) => {
  if (isExternal || health === undefined || health === null) {
    return { icon: 'status-unmanaged', text: t('card.unknown') };
  }
  if (health) {
    return { icon: 'status-schedulable', text: t('card.normal') };
  }
  return { icon: 'status-unschedulable', text: t('card.abnormal') };
};

const getRemainingTotalText = ({ total, used, unit = '', divisor = 1 }) => {
  const totalNum = Number(total || 0);
  if (!totalNum) return null;
  const usedNum = Number(used || 0);
  const normalizedDivisor = Number(divisor) > 0 ? Number(divisor) : 1;
  const remaining = Math.max(0, totalNum - usedNum);
  const unitText = unit ? ` ${unit}` : '';
  return {
    remaining: roundToDecimal(remaining / normalizedDivisor, 1),
    total: roundToDecimal(totalNum / normalizedDivisor, 1),
    unitText,
  };
};

const baseColumns = computed(() => [
  {
    title: t('card.uuid'),
    dataIndex: 'uuid',
    width: 250,
    hideTooltip: true,
    render: ({ uuid, type, vendor }) => {
      const to = `/accelerators/${uuid}`;
      const gpuModel = type || '';
      return (
        <div class="card-id-cell">
          <span class="card-id-cell-icon vgpu-table-name-icon-card">
            <svg-icon icon="vgpu-card" style={{ fontSize: '20px' }} />
          </span>
          <div class="card-id-cell-right">
            <span class="card-id-cell-name vgpu-table-name-text-wrap">
              <TextPlus
                text={uuid}
                to={to}
                tooltipClass="vgpu-long-text-tooltip vgpu-single-line-tooltip"
              />
            </span>
            {gpuModel && <p class="card-id-cell-model">{deviceWording(t('card.typeLabel'), vendor)}{gpuModel}</p>}
          </div>
        </div>
      );
    },
  },
  {
    title: t('task.status'),
    dataIndex: 'health',
    width: 150,
    render: ({ health, isExternal, unconfigured }) => {
      const { icon, text } = getCardStatusDisplay({ health, isExternal });
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
          {unconfigured ? <UnconfiguredTag /> : null}
        </span>
      );
    },
  },
  {
    title: t('card.splitMode.label'),
    dataIndex: 'mode',
    width: 170,
    render: ({ mode }) => {
      const key = getSplitModeKey(mode);
      if (!key) return <span>--</span>;
      return (
        <span class="card-split-mode">
          <svg-icon icon={getSplitIcon(mode)} aria-hidden="true" />
          <span>{t(key)}</span>
        </span>
      );
    },
  },
  {
    title: t('card.node'),
    dataIndex: 'nodeName',
    width: 200,
    hideTooltip: true,
    render: ({ nodeName }) => (
      <EllipsisText text={nodeName || '--'} mode="middle" tooltip="always" />
    ),
  },
  {
    title: t('card.computeRemainingTotal'),
    key: 'card-compute-remaining-total',
    dataIndex: 'used',
    width: 220,
    render: ({ coreTotal, coreUsed, coreUsedKnown, isExternal, unconfigured }) => {
      if (isExternal || unconfigured || coreUsedKnown === false || !coreTotal) return <span>--</span>;
      const stats = getRemainingTotalText({ total: coreTotal, used: coreUsed, divisor: 100 });
      if (!stats) return <span>--</span>;
      return (
        <div class="card-remaining-statistics">
          <span class="count">
            <span class="remaining">{stats.remaining}</span>/{stats.total}
          </span>
          <span class="tip">{t('card.allocatableTotal')}</span>
        </div>
      );
    },
  },
  {
    title: t('card.computeAllocTotal'),
    key: 'card-compute-allocation',
    dataIndex: 'used',
    width: 180,
    // The API still reports a registered size for an unconfigured device, which HAMi does not schedule.
    render: ({ coreTotal, coreUsed, coreUsedKnown, isExternal, unconfigured }) => {
      if (isExternal || unconfigured || coreUsedKnown === false || !coreTotal) return <span>--</span>;
      const percent = getAllocationPercent(coreUsed, coreTotal);
      if (!percent) return <span>--</span>;
      const color = getResourceColor(percent.progress);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
          <t-progress
            theme="circle"
            size={24}
            strokeWidth={3}
            percentage={roundToDecimal(percent.progress, 2)}
            color={color}
            label={false}
          />
          <span>{roundToDecimal(percent.raw, 2)}%</span>
        </span>
      );
    },
  },
  {
    title: t('card.memoryRemainingTotal'),
    key: 'card-memory-remaining-total',
    dataIndex: 'used',
    width: 220,
    render: ({ memoryTotal, memoryUsed, isExternal, unconfigured }) => {
      if (isExternal || unconfigured || !memoryTotal) return <span>--</span>;
      const stats = getRemainingTotalText({
        total: Number(memoryTotal) / 1024,
        used: Number(memoryUsed) / 1024,
        unit: 'GiB',
      });
      if (!stats) return <span>--</span>;
      return (
        <div class="card-remaining-statistics">
          <span class="count">
            <span class="remaining">{stats.remaining}</span>/{stats.total}{stats.unitText}
          </span>
          <span class="tip">{t('card.allocatableTotal')}</span>
        </div>
      );
    },
  },
  {
    title: t('card.memoryAllocTotal'),
    key: 'card-memory-allocation',
    dataIndex: 'w',
    width: 180,
    render: ({ memoryTotal, memoryUsed, isExternal, unconfigured }) => {
      if (isExternal || unconfigured || !memoryTotal) return <span>--</span>;
      const percent = getAllocationPercent(memoryUsed, memoryTotal);
      if (!percent) return <span>--</span>;
      const color = getResourceColor(percent.progress);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
          <t-progress
            theme="circle"
            size={24}
            strokeWidth={3}
            percentage={roundToDecimal(percent.progress, 2)}
            color={color}
            label={false}
          />
          <span>{roundToDecimal(percent.raw, 2)}%</span>
        </span>
      );
    },
  },
]);
const { eyeColumnKeys, columnOptions, visibleColumns } = useTableColumnVisibility(baseColumns);

const PieRef = ref();

const handlePieClick = (params, echarts) => {
  PieRef.value = echarts;
  const name = params.data.name;
  if (filters.type === name) {
    echarts.dispatchAction({
      type: 'downplay',
      seriesIndex: 0,
    });
    filters.type = undefined;
    applyFilters();
    return;
  }

  echarts.dispatchAction({
    type: 'downplay',
    seriesIndex: 0,
  });

  echarts.dispatchAction({
    type: 'highlight',
    seriesIndex: 0,
    dataIndex: params.dataIndex,
  });
  filters.type = name;
  pruneNodeScopeByType();
  applyFilters();
};

watch(
  () => filters.type,
  (newVal) => {
    if (!PieRef.value) return;
    const data = PieRef.value.getOption().series[0].data;
    if (newVal) {
      PieRef.value.dispatchAction({
        type: 'downplay',
        seriesIndex: 0,
      });

      PieRef.value.dispatchAction({
        type: 'highlight',
        seriesIndex: 0,
        dataIndex: data.findIndex((item) => item.name === newVal),
      });
    } else {
      PieRef.value.dispatchAction({
        type: 'downplay',
        seriesIndex: 0,
      });
    }
  },
);

// Embedded lists keep their fixed server filters; the toolbar filters run locally while typing.
const tableState = useFetchList(() => {
  const { nodeName, type, ...baseFilters } = props.filters || {};
  return request(cardApi.getCardList({ filters: baseFilters }));
}, { immediate: false });
const {
  data: tableData,
  refresh: fetchList,
  refreshError: tableRefreshError,
  refreshing: tableRefreshing,
  status: tableStatus,
} = tableState;
const refreshTable = fetchList;
const scopedNodeName = computed(() => (hasManualNodeScope.value ? filters.nodeName : props.filters?.nodeName));
const keywords = computed(() => parseKeywords(filters.keyword));
const keywordCards = computed(() => tableData.value.filter((card) =>
  matchesKeywords(keywords.value, [card.uuid, card.type, card.nodeName, card.vendor])));
const visibleCards = computed(() => keywordCards.value.filter((card) =>
  (!scopedNodeName.value || card.nodeName === scopedNodeName.value)
  && (!filters.type || card.type === filters.type)));
// Options show how many cards picking them would list under the other filters.
const countBy = (cards, keyOf) => {
  const counts = new Map();
  for (const card of cards) counts.set(keyOf(card), (counts.get(keyOf(card)) ?? 0) + 1);
  return counts;
};
const nodeCardCounts = computed(() => countBy(keywordCards.value.filter((card) => !filters.type || card.type === filters.type), (card) => card.nodeName));
const typeCardCount = computed(() => {
  const all = countBy(keywordCards.value, (card) => card.type);
  const onNode = scopedNodeName.value ? countBy(keywordCards.value.filter((card) => card.nodeName === scopedNodeName.value), (card) => card.type) : all;
  const node = rawNodes.value.find((item) => item.name === filters.nodeName);
  // Picking a type drops a node that does not carry it, as pruneNodeScopeByType does.
  const keepsNode = (type) => !filters.nodeName || !nodesLoaded.value || Boolean(node?.types.includes(type));
  return (type) => ((keepsNode(type) ? onNode : all).get(type) ?? 0);
});
// Counts stay hidden until the list loads; a loaded empty list still shows 0.
const cardCount = (count = 0) => (tableStatus.value === REQUEST_STATUS.READY ? { count, countLabel: t('card.deviceCount', { count }, count) } : {});
const filtered = computed(() => Boolean(
  keywords.value.length
  || (filters.type || '') !== (props.filters?.type || '')
  || (scopedNodeName.value || '') !== (props.filters?.nodeName || '')
));
const clearFilters = () => {
  filters.keyword = '';
  filters.type = props.filters?.type;
  filters.nodeName = props.filters?.nodeName;
  hasManualNodeScope.value = false;
};
const { pagination, pagedTableData, syncTotalAndClamp, resetToFirstPage } = useLocalPagination(visibleCards);
// A page restored from the URL waits for the list before it is clamped.
watch([visibleCards, tableStatus], () => {
  if (tableStatus.value === REQUEST_STATUS.READY) syncTotalAndClamp();
  else pagination.total = visibleCards.value.length;
}, { immediate: true, flush: 'sync' });
const applyFilters = resetToFirstPage;
const filterKey = computed(() => JSON.stringify([keywords.value, filters.type, scopedNodeName.value]));
let restoredFilterKey;
// Changing a filter starts from the first page; restoring one from the URL keeps its page.
watch(filterKey, (key) => {
  if (key !== restoredFilterKey) resetToFirstPage();
  restoredFilterKey = undefined;
});
const onNodeNameChange = () => {
  hasManualNodeScope.value = true;
  applyFilters();
};

const syncsRoute = !props.hideTitle && !props.filters;
const ROUTE_KEYS = ['q', 'type', 'nodeName', 'page', 'pageSize'];
const DEFAULT_PAGE_SIZE = 10;
watch(
  () => route.query,
  (query) => {
    if (!syncsRoute) return;
    const values = readQuery(query, ROUTE_KEYS);
    // Keep the typed text, including spaces between words, when the query only trimmed it.
    if ((values.q ?? '') !== filters.keyword.trim()) filters.keyword = values.q ?? '';
    filters.type = values.type;
    filters.nodeName = values.nodeName;
    hasManualNodeScope.value = Boolean(values.nodeName);
    const page = Number(values.page);
    const pageSize = Number(values.pageSize);
    pagination.pageSize = pagination.pageSizeOptions.includes(pageSize) ? pageSize : DEFAULT_PAGE_SIZE;
    pagination.current = Number.isInteger(page) && page > 0 ? page : 1;
    restoredFilterKey = filterKey.value;
  },
  { immediate: true },
);
watch(
  () => [filters.keyword, filters.type, filters.nodeName, pagination.current, pagination.pageSize],
  () => {
    if (!syncsRoute) return;
    const query = writeQuery(route.query, {
      q: filters.keyword.trim(),
      type: filters.type,
      nodeName: filters.nodeName,
      page: pagination.current > 1 ? pagination.current : undefined,
      pageSize: pagination.pageSize !== DEFAULT_PAGE_SIZE ? pagination.pageSize : undefined,
    });
    if (!sameQuery(query, route.query)) router.replace({ query });
  },
);

watch(
  () => [
    props.filters?.uid,
    props.filters?.type,
    props.filters?.nodeName,
  ],
  () => {
    hasManualNodeScope.value = false;
    filters.keyword = '';
    filters.type = props.filters?.type;
    filters.nodeName = props.filters?.nodeName;
    applyFilters();
  },
);

onMounted(() => {
  fetchFilterOptions();
  refreshTable();
});
</script>

<style scoped lang="scss">
.card-admin-page {
  &.is-embedded {
    .card-admin-table-wrap {
      margin-top: 15px;
    }
  }

  :deep(.preview) {
    margin-bottom: 0;
  }
}

.card-admin-top-wrap {
  display: flex;
  flex-direction: column;
  overflow: visible;
}

.card-admin-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 8px;

  :deep(.accelerator-table) {
    margin-top: 8px;
  }

  :deep(.card-remaining-statistics) {
    display: inline-flex;
    flex-direction: column;
    gap: 2px;

    .count {
      color: #939ea9;
      font-size: 12px;
      font-weight: 500;
      line-height: 18px;

      .remaining {
        color: #324558;
        font-size: 14px;
      }
    }

    .tip {
      color: #939ea9;
      font-size: 12px;
      font-weight: 400;
      line-height: 16px;
    }
  }
}

:deep(.card-id-cell) {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
}

:deep(.card-id-cell-right) {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

:deep(.card-id-cell-name) {
  display: flex;
  line-height: 20px;
}

:deep(.card-split-mode) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;

  svg {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
  }
}

:deep(.card-id-cell-model) {
  margin: 0;
  color: #939ea9;
  font-size: 12px;
  font-weight: 400;
  line-height: 16px;
}
</style>
