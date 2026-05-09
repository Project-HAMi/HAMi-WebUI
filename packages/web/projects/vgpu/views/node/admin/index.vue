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
        @refresh="refreshTable"
      >
        <t-space :size="8">
          <t-select
            v-model="filters.isSchedulable"
            clearable
            :placeholder="$t('node.allStatus')"
            :options="statusOptions"
            @change="applyFilters"
          />
          <t-select
            v-model="filters.type"
            clearable
            :placeholder="$t('node.allTypes')"
            :options="cardTypeOptions"
            @change="applyFilters"
          />
          <t-input
            v-model="filters.ip"
            clearable
            :placeholder="$t('node.searchIP')"
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
        :key="locale"
        row-key="uid"
        class="node-table vgpu-table-skin"
        :data="tableData"
        :columns="visibleColumns"
        :loading="tableLoading"
        table-layout="auto"
      />
    </div>
  </div>
</template>

<script setup lang="jsx">
import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';
import { useRouter, useRoute } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import { MessagePlugin } from 'tdesign-vue-next';
import request from '@/utils/request';
import { SearchIcon } from 'tdesign-icons-vue-next';
import { computed, ref, reactive, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import useTableFilters from '~/vgpu/hooks/useTableFilters';

const router = useRouter();
const route = useRoute();
const { t, locale } = useI18n();
const tableData = ref([]);
const tableLoading = ref(false);
const allNodeMap = ref(new Map());
const filters = reactive({
  isSchedulable: undefined,
  type: undefined,
  ip: '',
});
const rawCardTypes = ref([]);
const statusOptions = computed(() => [
  { label: t('node.allStatus'), value: undefined },
  { label: t('node.normal'), value: 'true' },
  { label: t('node.abnormal'), value: 'false' },
]);
const cardTypeOptions = computed(() => [
  { label: t('node.allTypes'), value: undefined },
  ...rawCardTypes.value.map((type) => ({ label: type, value: type })),
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
    router.push(`/admin/vgpu/node/admin/${uuid}?nodeName=${name}`);
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

const getNodeStatusDisplay = ({ isSchedulable, isExternal }) => {
  if (isExternal || isSchedulable === undefined || isSchedulable === null) {
    return { icon: 'status-unmanaged', text: t('node.unknown') };
  }
  if (isSchedulable) {
    return { icon: 'status-schedulable', text: t('node.normal') };
  }
  return { icon: 'status-unschedulable', text: t('node.abnormal') };
};

const baseColumns = computed(() => [
  {
    title: t('node.name'),
    minWidth: 200,
    dataIndex: 'name',
    render: ({ uid, name }) => {
      const to = `/admin/vgpu/node/admin/${uid}?nodeName=${name}`;
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <span class="vgpu-table-name-icon-card node-name-icon-card" onClick={() => router.push(to)}>
            <svg-icon icon="node-name" style={{ fontSize: '20px' }} />
          </span>
          <span class="vgpu-table-name-text-wrap">
            <text-plus text={name} to={to} />
          </span>
        </span>
      );
    },
  },
  {
    title: t('task.status'),
    minWidth: 150,
    dataIndex: 'isSchedulable',
    render: ({ isSchedulable, isExternal }) => {
      const { icon, text } = getNodeStatusDisplay({ isSchedulable, isExternal });
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
        </span>
      );
    },
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
    render: ({ coreTotal, coreUsed, isExternal }) => {
      if (isExternal || !coreTotal) return <span>--</span>;
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

const fetchTableData = async () => {
  tableLoading.value = true;
  try {
    const payload = {
      filters: {
        ...(filters.isSchedulable ? { isSchedulable: filters.isSchedulable } : {}),
        ...(filters.type ? { type: filters.type } : {}),
        ...(getTrimValue(filters.ip) ? { ip: getTrimValue(filters.ip) } : {}),
      },
    };
    const { list = [] } = await request(nodeApi.getNodeList(payload));
    tableData.value = list;
  } finally {
    tableLoading.value = false;
  }
};
const { getTrimValue, applyFilters, refreshTable } = useTableFilters({ fetchTableData });

const parseSchedulableFromQuery = (value) => {
  if (value === 'true' || value === 'false') return value;
  return undefined;
};

let hasInitializedByRoute = false;
watch(
  () => route.query.isSchedulable,
  (value) => {
    const next = parseSchedulableFromQuery(value);
    if (filters.isSchedulable === next && hasInitializedByRoute) return;
    hasInitializedByRoute = true;
    filters.isSchedulable = next;
    applyFilters();
  },
  { immediate: true },
);

onMounted(() => {
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
  cursor: pointer;
  user-select: none;
}

:deep(.vgpu-table-name-text-wrap .text-plus) {
  align-items: center;
}

:deep(.vgpu-table-name-text-wrap) {
  flex: 1;
}
</style>
