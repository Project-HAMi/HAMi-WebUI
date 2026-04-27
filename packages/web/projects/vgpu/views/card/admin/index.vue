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
const { t, locale } = useI18n();
const isEnglish = computed(() => String(locale.value || '').startsWith('en'));
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
});
const rawNodeNames = ref([]);
const rawCardTypes = ref([]);
const nodeOptions = computed(() => [
  { label: t('card.allNodes'), value: undefined },
  ...rawNodeNames.value.map((name) => ({ label: name, value: name })),
]);
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
    rawNodeNames.value = nodeList
      .map((item) => item?.name)
      .filter(Boolean);
    rawCardTypes.value = typeList
      .map((item) => item?.type)
      .filter(Boolean);
  } catch {
    rawNodeNames.value = [];
    rawCardTypes.value = [];
  }
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

const getRemainingTotalText = ({ total, used, unit = '' }) => {
  const totalNum = Number(total || 0);
  if (!totalNum) return null;
  const usedNum = Number(used || 0);
  const remaining = Math.max(0, totalNum - usedNum);
  const unitText = unit ? ` ${unit}` : '';
  return {
    remaining: roundToDecimal(remaining, 1),
    total: roundToDecimal(totalNum, 1),
    unitText,
  };
};

const baseColumns = computed(() => [
  {
    title: t('card.uuid'),
    dataIndex: 'uuid',
    width: 250,
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
    width: 100,
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
    title: t('card.mode'),
    dataIndex: 'mode',
    width: 120,
    render: ({ mode, type }) => (
        <t-tag theme="default" variant="light">
          {type?.split('-')[0] === "NVIDIA" ? mode : 'default'}
        </t-tag>
    )
  },
  {
    title: t('card.node'),
    dataIndex: 'nodeName',
    width: 170,
    hideTooltip: true,
    render: ({ nodeName }) => (
      <ellipsis-text text={nodeName || '--'} mode="middle" tooltip="always" />
    ),
  },
  {
    title: t('card.vgpu'),
    key: 'card-vgpu',
    dataIndex: 'used',
    width: 100,
    render: ({ vgpuTotal, vgpuUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : vgpuUsed}/{isExternal ? '--' : vgpuTotal}
    </span>
    ),
  },
  {
    title: t('card.computeRemainingTotal'),
    key: 'card-compute-remaining-total',
    dataIndex: 'used',
    width: isEnglish.value ? 220 : 180,
    render: ({ coreTotal, coreUsed, isExternal }) => {
      if (isExternal || !coreTotal) return <span>--</span>;
      const stats = getRemainingTotalText({ total: coreTotal, used: coreUsed });
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
    width: isEnglish.value ? 170 : 140,
    render: ({ coreTotal, coreUsed, isExternal }) => {
      if (isExternal || !coreTotal) return <span>--</span>;
      const percent = Math.max(
        0,
        Math.min(100, (Number(coreUsed || 0) / Number(coreTotal)) * 100),
      );
      const color = getResourceColor(percent);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
          <t-progress
            theme="circle"
            size={24}
            strokeWidth={3}
            percentage={roundToDecimal(percent, 2)}
            color={color}
            label={false}
          />
          <span>{roundToDecimal(percent, 2)}%</span>
        </span>
      );
    },
  },
  {
    title: t('card.memoryRemainingTotal'),
    key: 'card-memory-remaining-total',
    dataIndex: 'used',
    width: isEnglish.value ? 210 : 180,
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
    title: t('card.memoryAllocTotal'),
    key: 'card-memory-allocation',
    dataIndex: 'w',
    width: isEnglish.value ? 160 : 140,
    render: ({ memoryTotal, memoryUsed, isExternal }) => {
      if (isExternal || !memoryTotal) return <span>--</span>;
      const percent = Math.max(
        0,
        Math.min(100, (Number(memoryUsed || 0) / Number(memoryTotal)) * 100),
      );
      const color = getResourceColor(percent);
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '8px' }}>
          <t-progress
            theme="circle"
            size={24}
            strokeWidth={3}
            percentage={roundToDecimal(percent, 2)}
            color={color}
            label={false}
          />
          <span>{roundToDecimal(percent, 2)}%</span>
        </span>
      );
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
