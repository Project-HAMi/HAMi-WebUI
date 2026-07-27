<template>
  <div class="card-admin-page vgpu-admin-page" :class="{ 'is-embedded': hideTitle }">
    <div v-if="!hideTitle" class="vgpu-admin-page-title">{{ $t('card.title') }}</div>

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
        @refresh="refreshTable"
      >
        <t-space :size="8">
          <t-select
            v-model="filters.type"
            clearable
            :placeholder="$t('card.allCardTypes')"
            :options="cardTypeOptions"
            @change="onTypeChange"
          />
          <t-select
            v-model="filters.health"
            clearable
            :placeholder="$t('card.allStatus')"
            :options="statusOptions"
            @change="applyFilters"
          />
          <t-select
            v-model="filters.nodeName"
            clearable
            :placeholder="$t('card.allNodes')"
            :options="nodeOptions"
            @change="onNodeNameChange"
          />
          <t-input
            v-model="filters.uid"
            clearable
            :placeholder="$t('card.searchByName')"
            @enter="applyFilters"
            @blur="applyFilters"
          >
            <template #prefix-icon>
              <search-icon :style="{ cursor: 'pointer' }" />
            </template>
          </t-input>
        </t-space>
      </toolbar>
      <t-table
        :key="`${locale}-card-table`"
        row-key="uuid"
        class="accelerator-table vgpu-table-skin"
        :data="pagedTableData"
        :columns="visibleColumns"
        :loading="tableLoading"
        table-layout="fixed"
      />
      <table-pagination
        :total="pagination.total"
        :current="pagination.current"
        :page-size="pagination.pageSize"
        :page-sizes="pagination.pageSizeOptions"
        :show-jumper="pagination.showJumper"
        @update:current="(val) => (pagination.current = val)"
        @update:pageSize="(val) => (pagination.pageSize = val)"
      />
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
import request from '@/utils/request';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { ref, watch, computed, reactive, onMounted } from 'vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import useTableFilters from '~/vgpu/hooks/useTableFilters';
import useLocalPagination from '~/vgpu/hooks/useLocalPagination';

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
const tableData = ref([]);
const tableLoading = ref(false);
const hasManualNodeScope = ref(false);
const filters = reactive({
  uid: '',
  nodeName: props.filters?.nodeName,
  type: props.filters?.type ?? parseTypeFromQuery(route.query.type),
  health: undefined,
});
const rawNodes = ref([]);
const rawCardTypes = ref([]);
const statusOptions = computed(() => [
  { label: t('card.allStatus'), value: undefined },
  { label: t('card.normal'), value: 'true' },
  { label: t('card.abnormal'), value: 'false' },
]);
const nodeOptions = computed(() => {
  // When a card type is selected, only offer nodes that actually carry that type.
  const nodes = filters.type
    ? rawNodes.value.filter((node) => node.types.includes(filters.type))
    : rawNodes.value;
  return [
    { label: t('card.allNodes'), value: undefined },
    ...nodes.map((node) => ({ label: node.name, value: node.name })),
  ];
});
const cardTypeOptions = computed(() => [
  { label: t('card.allCardTypes'), value: undefined },
  ...rawCardTypes.value.map((type) => ({ label: type, value: type })),
]);

const fetchFilterOptions = async () => {
  try {
    const [{ list: nodeList = [] }, { list: typeList = [] }] = await Promise.all([
      request(nodeApi.getNodeList({ filters: {} })),
      request(cardApi.getCardType()),
    ]);
    rawNodes.value = nodeList
      .filter((item) => item?.name)
      .map((item) => ({ name: item.name, types: Array.isArray(item.type) ? item.type : [] }));
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
  if (!filters.type || !filters.nodeName) {
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
    path: `/admin/vgpu/card/admin/${params.data.name}`,
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

const renderPercentPair = ({ allocPercent, usagePercent }) => {
  const clamp = (v) => Math.max(0, Math.min(100, v));
  const formatPercent = (v) => {
    const n = Number.isFinite(v) ? v : 0;
    return n.toFixed(2);
  };
  const allocRaw = Number.isFinite(allocPercent) ? allocPercent : 0;
  const usageRaw = Number.isFinite(usagePercent) ? usagePercent : 0;
  const allocProgress = clamp(allocRaw);
  const usageProgress = clamp(usageRaw);
  return (
    <span class="card-resource-pair">
      <span class="card-resource-item">
        <t-progress
          theme="circle"
          size={24}
          strokeWidth={3}
          percentage={Number(formatPercent(allocProgress))}
          color={getResourceColor(allocProgress)}
          label={false}
        />
        <span>{formatPercent(allocRaw)}%</span>
      </span>
      <span class="card-resource-item">
        <t-progress
          theme="circle"
          size={24}
          strokeWidth={3}
          percentage={Number(formatPercent(usageProgress))}
          color={getResourceColor(usageProgress)}
          label={false}
        />
        <span>{formatPercent(usageRaw)}%</span>
      </span>
    </span>
  );
};

const baseColumns = computed(() => [
  {
    title: t('card.uuid'),
    dataIndex: 'uuid',
    width: 220,
    hideTooltip: true,
    render: ({ uuid, type }) => {
      const to = `/admin/vgpu/card/admin/${uuid}`;
      const gpuModel = type || '';
      return (
        <div class="card-id-cell">
          <span class="card-id-cell-icon vgpu-table-name-icon-card" onClick={() => router.push(to)}>
            <svg-icon icon="card-id" style={{ fontSize: '20px' }} />
          </span>
          <div class="card-id-cell-right">
            <span class="card-id-cell-name vgpu-table-name-text-wrap">
              <text-plus text={uuid} to={to} />
            </span>
            {gpuModel && <p class="card-id-cell-model">{t('card.typeLabel')}{gpuModel}</p>}
          </div>
        </div>
      );
    },
  },
  {
    title: t('task.status'),
    dataIndex: 'health',
    width: 110,
    render: ({ health, isExternal }) => {
      const { icon, text } = getCardStatusDisplay({ health, isExternal });
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
        </span>
      );
    },
  },
  {
    title: t('card.node'),
    dataIndex: 'nodeName',
    width: 160,
    hideTooltip: true,
    render: ({ nodeName, nodeIp }) => (
      <span class="card-node-cell">
        <ellipsis-text text={nodeName || '--'} mode="middle" tooltip="always" />
        <span class="card-node-ip">{nodeIp || '--'}</span>
      </span>
    ),
  },
  {
    title: t('card.computeRemainingTotal'),
    key: 'card-compute-remaining-total',
    dataIndex: 'used',
    width: 170,
    render: ({ coreTotal, coreUsed, isExternal }) => {
      if (isExternal || !coreTotal) return <span>--</span>;
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
    title: `${t('card.computeAllocTotal')}/${t('card.computeUsage')}`,
    key: 'card-compute-allocation',
    dataIndex: 'coreUsed',
    width: 210,
    render: ({ coreTotal, coreUsed, coreUsage, isExternal }) => {
      if (isExternal || !coreTotal) return <span>--</span>;
      const allocPercent = (Number(coreUsed || 0) / Number(coreTotal)) * 100;
      const usagePercent = Number(coreUsage || 0);
      return renderPercentPair({ allocPercent, usagePercent });
    },
  },
  {
    title: t('card.memoryRemainingTotal'),
    key: 'card-memory-remaining-total',
    dataIndex: 'used',
    width: 170,
    render: ({ memoryTotal, memoryUsed, isExternal }) => {
      if (isExternal || !memoryTotal) return <span>--</span>;
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
    title: `${t('card.memoryAllocTotal')}/${t('card.memoryUsage')}`,
    key: 'card-memory-allocation',
    dataIndex: 'memoryUsed',
    width: 210,
    render: ({ memoryTotal, memoryUsed, memoryUsage, isExternal }) => {
      if (isExternal || !memoryTotal) return <span>--</span>;
      const allocPercent = (Number(memoryUsed || 0) / Number(memoryTotal)) * 100;
      const usagePercent = (Number(memoryUsage || 0) / Number(memoryTotal)) * 100;
      return renderPercentPair({ allocPercent, usagePercent });
    },
  },
]);
const { eyeColumnKeys, columnOptions, visibleColumns } = useTableColumnVisibility(baseColumns);
const { pagination, pagedTableData, syncTotalAndClamp, resetToFirstPage } = useLocalPagination(tableData);

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

const fetchTableData = async () => {
  tableLoading.value = true;
  try {
    const baseFilters = { ...(props.filters || {}) };
    delete baseFilters.nodeName;
    const nodeName = hasManualNodeScope.value ? filters.nodeName : props.filters?.nodeName;
    const payload = {
      filters: {
        ...baseFilters,
        ...(getTrimValue(filters.uid) ? { uid: getTrimValue(filters.uid) } : {}),
        ...(nodeName ? { nodeName } : {}),
        ...(filters.type ? { type: filters.type } : {}),
        ...(filters.health ? { health: filters.health } : {}),
      },
    };
    const { list = [] } = await request(cardApi.getCardList(payload));
    tableData.value = list;
    syncTotalAndClamp();
  } finally {
    tableLoading.value = false;
  }
};
const { getTrimValue, applyFilters, refreshTable } = useTableFilters({
  fetchTableData,
  resetBeforeApply: resetToFirstPage,
});
const onNodeNameChange = () => {
  hasManualNodeScope.value = true;
  applyFilters();
};

watch(
  () => route.query.type,
  (value) => {
    const next = parseTypeFromQuery(value);
    if (filters.type === next) return;
    filters.type = next;
    pruneNodeScopeByType();
    applyFilters();
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
    filters.uid = props.filters?.uid || '';
    filters.type = props.filters?.type;
    filters.nodeName = props.filters?.nodeName;
    applyFilters();
  },
);

onMounted(() => {
  fetchFilterOptions();
  applyFilters();
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

:deep(.card-id-cell-icon) {
  cursor: pointer;
}

:deep(.card-id-cell-right) {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

:deep(.card-node-cell) {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}

:deep(.card-node-ip) {
  color: #939ea9;
  font-size: 12px;
  font-weight: 400;
  line-height: 16px;
}

:deep(.card-resource-pair) {
  display: inline-flex;
  align-items: center;
  gap: 12px;
}

:deep(.card-resource-item) {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

:deep(.card-id-cell-name) {
  display: flex;
}

:deep(.card-id-cell-model) {
  margin: 0;
  color: #939ea9;
  font-size: 12px;
  font-weight: 400;
  line-height: 1.4;
}
</style>
