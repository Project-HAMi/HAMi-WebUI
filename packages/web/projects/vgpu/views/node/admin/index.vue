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
            v-model="filters.role"
            clearable
            :placeholder="$t('node.allTypes')"
            :options="roleOptions"
            @change="applyFilters"
          />
          <t-input
            v-model="filters.keyword"
            clearable
            :placeholder="$t('node.searchNameOrIP')"
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
import { useRouter, useRoute } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import { getResourceColor } from '@/utils';
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
  role: undefined,
  keyword: '',
});
const statusOptions = computed(() => [
  { label: t('node.allStatus'), value: undefined },
  { label: t('node.normal'), value: 'true' },
  { label: t('node.abnormal'), value: 'false' },
]);
const roleOptions = computed(() => [
  { label: t('node.allTypes'), value: undefined },
  { label: t('node.controlPlane'), value: 'control-plane' },
  { label: t('node.workerNode'), value: 'worker' },
]);

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

const getNodeRoleText = (role) => {
  if (role === 'control-plane') return t('node.controlPlane');
  if (role === 'worker') return t('node.workerNode');
  return t('node.unknown');
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
    <span class="node-resource-pair">
      <span class="node-resource-item">
        <t-progress
          class="node-admin-circle-progress"
          theme="circle"
          size={24}
          strokeWidth={3}
          percentage={Number(formatPercent(allocProgress))}
          color={getResourceColor(allocProgress)}
          label={false}
        />
        <span>{formatPercent(allocRaw)}%</span>
      </span>
      <span class="node-resource-item">
        <t-progress
          class="node-admin-circle-progress"
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
    title: t('node.name'),
    minWidth: 200,
    dataIndex: 'name',
    render: ({ uid, name, role }) => {
      const to = `/admin/vgpu/node/admin/${uid}?nodeName=${name}`;
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <span class="vgpu-table-name-icon-card node-name-icon-card" onClick={() => router.push(to)}>
            <svg-icon icon="node-name" style={{ fontSize: '20px' }} />
          </span>
          <span class="vgpu-table-name-text-wrap">
            <text-plus text={name} to={to} />
            <span class="node-role-text">
              {t('node.nodeRolePrefix')}
              {getNodeRoleText(role)}
            </span>
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
    title: `${t('node.computeAllocTotal')}/${t('node.computeUsage')}`,
    key: 'node-compute-allocation',
    minWidth: 280,
    dataIndex: 'coreUsed',
    render: ({ coreTotal, coreUsed, coreUsage, isExternal }) => {
      if (isExternal || !coreTotal) return <span>--</span>;
      const allocPercent = (Number(coreUsed || 0) / Number(coreTotal)) * 100;
      const usagePercent = Number(coreUsage || 0);
      return renderPercentPair({ allocPercent, usagePercent });
    },
  },
  {
    title: `${t('node.memoryAllocTotal')}/${t('node.memoryUsage')}`,
    key: 'node-memory-allocation',
    minWidth: 280,
    dataIndex: 'memoryUsed',
    render: ({ memoryTotal, memoryUsed, memoryUsage, isExternal }) => {
      if (isExternal || !memoryTotal) return <span>--</span>;
      const allocPercent = (Number(memoryUsed || 0) / Number(memoryTotal)) * 100;
      const usagePercent = (Number(memoryUsage || 0) / Number(memoryTotal)) * 100;
      return renderPercentPair({ allocPercent, usagePercent });
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
        ...(filters.role ? { role: filters.role } : {}),
        ...(getTrimValue(filters.keyword) ? { keyword: getTrimValue(filters.keyword) } : {}),
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
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 0;
}

:deep(.node-role-text) {
  color: var(--td-text-color-secondary, #8b8b8b);
  font-size: 12px;
  line-height: 18px;
  text-align: left;
}

:deep(.node-resource-pair) {
  display: inline-flex;
  align-items: center;
  gap: 16px;
}

:deep(.node-resource-item) {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
</style>
