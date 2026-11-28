<template>
  <div class="card-admin-page vgpu-admin-page">
    <div class="vgpu-admin-page-title">{{ $t('card.title') }}</div>

    <DeviceConfigAlert />

    <div class="card-admin-top-wrap">
      <preview-bar
        :title="$t('dashboard.card')"
        type="device_uuid"
        :handle-click="handleClick"
        :handle-pie-click="handlePieClick"
        :currentName="currentType"
        :model-distribution="modelDistribution"
        :rankings="rankings"
        :ranking-status="tableStatus"
      />
    </div>

    <div class="card-admin-table-wrap">
      <toolbar
        v-model="eyeColumnKeys"
        :column-options="columnOptions"
        :refreshing="tableRefreshing"
        @refresh="refreshTable"
      >
        <div class="inventory-list-filters">
          <SegmentedControl
            v-model="filters.health"
            :options="healthOptions"
            :aria-label="$t('card.status')"
          />
          <t-select
            class="inventory-filter-select"
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
            class="inventory-filter-select"
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
            class="list-keyword-search inventory-filter-search"
            clearable
            :placeholder="$t('card.searchKeyword')"
            :aria-label="$t('card.searchKeyword')"
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
          row-key="physicalKey"
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
import { getPhysicalCardList } from '@/api/physical-inventory.mjs';
import apiClient from '@/api/client.mjs';
import { useRouter, useRoute } from 'vue-router';
import { cardHealth, cardHealthDisplay, healthLabels } from './card-health.mjs';
import VendorMark from '@/components/VendorMark.vue';
import PreviewBar from '@/components/previewBar.vue';
import SegmentedControl from '@/components/SegmentedControl/index.vue';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import TablePagination from '@/components/TablePlus/Pagination.vue';
import StatefulTable from '@/components/TablePlus/StatefulTable.vue';
import EllipsisText from '@/components/EllipsisText.vue';
import TextPlus from '@/components/TextPlus.vue';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { ref, watch, computed, reactive, onMounted, onUnmounted } from 'vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '@/hooks/useTableColumnVisibility';
import { readQuery, sameQuery, writeQuery } from '@/hooks/route-filters.mjs';
import { createOptionFilter, limitOptions, matchesKeywords, parseKeywords } from '@/utils/keyword-search.mjs';
import { fitPopupToOptions } from '@/utils/select-popup.mjs';
import SelectOptionLabel from '@/components/SelectOptionLabel.vue';
import useLocalPagination from '@/hooks/useLocalPagination';
import { physicalCardRankings } from './physical-card-rankings.mjs';
import { computeAllocationPercent, memoryAllocationPercent } from './allocation-percent.mjs';
import UnconfiguredTag from './components/UnconfiguredTag.vue';
import DeviceConfigAlert from '@/components/DeviceConfigAlert.vue';
import { deviceWording } from '@/components/device-copy.mjs';
import { getSplitIcon, getSplitModeKey } from '@/components/split-mode.mjs';
import useFetchList from '@/hooks/useFetchList';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';

const router = useRouter();
const route = useRoute();
const { t, locale } = useI18n();
const parseTypeFromQuery = (value) => {
  if (typeof value === 'string') return value || undefined;
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0] || undefined;
  return undefined;
};

const currentType = computed(() => filters.type || '');
const filters = reactive({
  keyword: '',
  health: '',
  nodeName: undefined,
  type: parseTypeFromQuery(route.query.type),
});
const registeredNodes = ref([]);
const optionsReady = computed(() => tableStatus.value === REQUEST_STATUS.READY);
const rawNodes = computed(() => {
  const ips = new Map(registeredNodes.value.map(n => [n.name, n.ip]));
  const models = new Map();
  for (const row of tableData.value) {
    if (!models.has(row.nodeName)) models.set(row.nodeName, new Set());
    models.get(row.nodeName).add(row.type);
  }
  return Array.from(models, ([name, types]) => ({ name, ip: ips.get(name), types: [...types] }));
});
const rawCardTypes = computed(() => [...new Set(tableData.value.map(row => row.type).filter(Boolean))].sort());
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

let filterOptionsController = null;
onUnmounted(() => {
  filterOptionsController?.abort();
  filterOptionsController = null;
});

const fetchFilterOptions = async () => {
  filterOptionsController?.abort();
  const controller = new AbortController();
  filterOptionsController = controller;
  try {
    const { list: nodeList = [] } = await apiClient.listNodes({}, { signal: controller.signal });
    if (controller.signal.aborted) return;
    registeredNodes.value = nodeList
      .filter((item) => item?.name)
      .map((item) => ({ name: item.name, ip: item.ip }));
  } catch {
    if (controller.signal.aborted) return;
    registeredNodes.value = [];
  } finally {
    if (filterOptionsController === controller) filterOptionsController = null;
  }
};

// When the selected type no longer includes the chosen node, drop the node so
// the table isn't filtered by an out-of-scope node. Call synchronously before
// applyFilters at every type-mutation site to avoid querying with a stale node.
const pruneNodeScopeByType = () => {
  // Unloaded or failed options must not drop a node restored from the URL.
  if (!filters.type || !filters.nodeName || !optionsReady.value) {
    return;
  }
  const node = rawNodes.value.find((item) => item.name === filters.nodeName);
  if (!node || !node.types.includes(filters.type)) {
    filters.nodeName = undefined;
  }
};

const onTypeChange = () => {
  pruneNodeScopeByType();
  applyFilters();
};

const handleClick = (params) => {
  if (params.data.detailTo) router.push(params.data.detailTo);
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
    render: ({ uuid, type, vendor, detailTo }) => {
      const gpuModel = type || '';
      return (
        <div class="card-id-cell vgpu-table-name-cell">
          <span class="card-id-cell-icon vgpu-table-name-icon-card">
            <VendorMark vendor={vendor} size={20}><svg-icon icon="vgpu-card" style={{ fontSize: '20px' }} /></VendorMark>
          </span>
          <div class="card-id-cell-right">
            <span class="card-id-cell-name vgpu-table-name-text-wrap">
              <TextPlus
                text={uuid}
                to={detailTo}
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
    render: (row) => {
      const { icon, text } = cardHealthDisplay(row, t);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
          {row.unconfigured ? <UnconfiguredTag /> : null}
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
    render: (card) => {
      const percent = computeAllocationPercent(card);
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
    render: (card) => {
      const percent = memoryAllocationPercent(card);
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

// Summary charts cover the full physical inventory. Toolbar filters apply only to the table.
const physicalModels = ref(null);
const tableState = useFetchList(() => getPhysicalCardList(), {
  immediate: false,
  onSuccess: reply => { physicalModels.value = reply.models; },
});
const {
  data: tableData,
  refresh: fetchList,
  refreshError: tableRefreshError,
  refreshing: tableRefreshing,
  status: tableStatus,
} = tableState;
const refreshTable = fetchList;
const scopedNodeName = computed(() => filters.nodeName);
const keywords = computed(() => parseKeywords(filters.keyword));
// Summary charts cover the full physical inventory. Toolbar filters apply only to the table.
const rankings = computed(() => physicalCardRankings(tableData.value));
const modelDistribution = computed(() => (physicalModels.value?.rows || []).map(row => ({
  name: row.type || '', value: row.count, vendor: row.vendor,
})));
const keywordCards = computed(() => tableData.value.filter((card) =>
  matchesKeywords(keywords.value, [card.uuid, card.type, card.nodeName, card.vendor])));
const scopedCards = computed(() => keywordCards.value.filter((card) =>
  (!scopedNodeName.value || card.nodeName === scopedNodeName.value)
  && (!filters.type || card.type === filters.type)));
const countBy = (cards, keyOf) => {
  const counts = new Map();
  for (const card of cards) counts.set(keyOf(card), (counts.get(keyOf(card)) ?? 0) + 1);
  return counts;
};
const matchesHealth = card => !filters.health || cardHealth(card) === filters.health;
const visibleCards = computed(() => scopedCards.value.filter(matchesHealth));
const healthOptions = computed(() => {
  const counts = countBy(scopedCards.value, cardHealth);
  const ready = tableStatus.value === REQUEST_STATUS.READY;
  return [
    { value: '', label: t('task.statusAll'), count: ready ? scopedCards.value.length : undefined },
    ...['healthy', 'unhealthy', 'unknown'].filter(key => key !== 'unknown' || counts.has(key) || filters.health === key)
      .map(key => ({ value: key, label: t(healthLabels[key]), count: ready ? counts.get(key) ?? 0 : undefined,
        tone: key === 'unhealthy' ? 'danger' : undefined })),
  ];
});
const healthFilteredCards = computed(() => keywordCards.value.filter(matchesHealth));
// Options show how many cards picking them would list under the other filters.
const nodeCardCounts = computed(() => countBy(healthFilteredCards.value.filter((card) => !filters.type || card.type === filters.type), (card) => card.nodeName));
const typeCardCount = computed(() => {
  const all = countBy(healthFilteredCards.value, (card) => card.type);
  const onNode = scopedNodeName.value ? countBy(healthFilteredCards.value.filter((card) => card.nodeName === scopedNodeName.value), (card) => card.type) : all;
  const node = rawNodes.value.find((item) => item.name === filters.nodeName);
  // Picking a type drops a node that does not carry it, as pruneNodeScopeByType does.
  const keepsNode = (type) => !filters.nodeName || !optionsReady.value || Boolean(node?.types.includes(type));
  return (type) => ((keepsNode(type) ? onNode : all).get(type) ?? 0);
});
// Counts stay hidden until the list loads; a loaded empty list still shows 0.
const cardCount = (count = 0) => (tableStatus.value === REQUEST_STATUS.READY ? { count, countLabel: t('card.deviceCount', { count }, count) } : {});
const filtered = computed(() => Boolean(
  keywords.value.length || filters.health || filters.type || scopedNodeName.value
));
const clearFilters = () => {
  filters.keyword = '';
  filters.health = '';
  filters.type = undefined;
  filters.nodeName = undefined;
};
const { pagination, pagedTableData, syncTotalAndClamp, resetToFirstPage } = useLocalPagination(visibleCards);
// A page restored from the URL waits for the list before it is clamped.
watch([visibleCards, tableStatus], () => {
  if (tableStatus.value === REQUEST_STATUS.READY) syncTotalAndClamp();
  else pagination.total = visibleCards.value.length;
}, { immediate: true, flush: 'sync' });
const applyFilters = resetToFirstPage;
const filterKey = computed(() => JSON.stringify([keywords.value, filters.health, filters.type, scopedNodeName.value]));
let restoredFilterKey;
// Changing a filter starts from the first page; restoring one from the URL keeps its page.
watch(filterKey, (key) => {
  if (key !== restoredFilterKey) resetToFirstPage();
  restoredFilterKey = undefined;
});
const onNodeNameChange = applyFilters;

const ROUTE_KEYS = ['q', 'type', 'nodeName', 'health', 'page', 'pageSize'];
const DEFAULT_PAGE_SIZE = 10;
watch(
  () => route.query,
  (query) => {
    const values = readQuery(query, ROUTE_KEYS);
    // Keep the typed text, including spaces between words, when the query only trimmed it.
    if ((values.q ?? '') !== filters.keyword.trim()) filters.keyword = values.q ?? '';
    filters.type = values.type;
    filters.health = Object.hasOwn(healthLabels, values.health) ? values.health : '';
    filters.nodeName = values.nodeName;
    const page = Number(values.page);
    const pageSize = Number(values.pageSize);
    pagination.pageSize = pagination.pageSizeOptions.includes(pageSize) ? pageSize : DEFAULT_PAGE_SIZE;
    pagination.current = Number.isInteger(page) && page > 0 ? page : 1;
    restoredFilterKey = filterKey.value;
  },
  { immediate: true },
);
watch(
  () => [filters.keyword, filters.health, filters.type, filters.nodeName, pagination.current, pagination.pageSize],
  () => {
    const query = writeQuery(route.query, {
      q: filters.keyword.trim(),
      health: filters.health || undefined,
      type: filters.type,
      nodeName: filters.nodeName,
      page: pagination.current > 1 ? pagination.current : undefined,
      pageSize: pagination.pageSize !== DEFAULT_PAGE_SIZE ? pagination.pageSize : undefined,
    });
    if (!sameQuery(query, route.query)) router.replace({ query });
  },
);

onMounted(() => {
  fetchFilterOptions();
  refreshTable();
});
</script>

<style scoped lang="scss">
.card-admin-page {
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

:deep(.card-id-cell-right) {
  min-width: 0;
  flex: 1;
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
