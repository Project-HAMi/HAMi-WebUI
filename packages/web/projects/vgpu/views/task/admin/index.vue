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
      <t-table
        :key="locale"
        row-key="podUid"
        class="workload-table vgpu-table-skin"
        :data="pagedTableData"
        :columns="visibleColumns"
        :loading="tableLoading"
        table-layout="auto"
        :style="style"
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

    <form-plus-drawer
      v-model="state.visible"
      v-model:form="state.formValues"
      :schema="state.schema"
      :title="state.title"
      @ok="state.ok"
    />
  </div>
</template>

<script setup lang="jsx">
import taskApi from '~/vgpu/api/task';
import nodeApi from '~/vgpu/api/node';
import Toolbar from '@/components/TablePlus/Toolbar.vue';
import TablePagination from '@/components/TablePlus/Pagination.vue';
import { roundToDecimal, timeParse } from '@/utils';
import request from '@/utils/request';
import { SearchIcon, HelpCircleIcon } from 'tdesign-icons-vue-next';
import { reactive, ref, computed, onMounted, watch } from 'vue';
import Top from './top.vue';
import { useI18n } from 'vue-i18n';
import useTableColumnVisibility from '~/vgpu/hooks/useTableColumnVisibility';
import useTableFilters from '~/vgpu/hooks/useTableFilters';
import useLocalPagination from '~/vgpu/hooks/useLocalPagination';

const props = defineProps(['hideTitle', 'filters', 'style']);
const { t, locale } = useI18n();
const tableData = ref([]);
const tableLoading = ref(false);
const hasManualNodeScope = ref(false);
const filters = reactive({
  name: props.filters?.name || '',
  nodeName: props.filters?.nodeName,
  status: props.filters?.status,
});
const rawNodeNames = ref([]);
const nodeOptions = computed(() => [
  { label: t('task.allNodes'), value: undefined },
  ...rawNodeNames.value.map((name) => ({ label: name, value: name })),
]);
const statusOptions = computed(() => [
  { label: t('task.allStatus'), value: undefined },
  { label: t('task.statusCompleted'), value: 'closed' },
  { label: t('task.statusRunning'), value: 'success' },
  { label: t('task.statusFailed'), value: 'failed' },
  { label: t('task.statusUnknown'), value: 'unknown' },
]);

const state = reactive({
  visible: false,
  schema: {},
  formValues: {},
  title: '',
  ok: () => {},
});

const fetchFilterOptions = async () => {
  try {
    const { list: nodeList = [] } = await request(nodeApi.getNodeList({ filters: {} }));
    rawNodeNames.value = nodeList
      .map((item) => item?.name)
      .filter(Boolean);
  } catch {
    rawNodeNames.value = [];
  }
};

const baseColumns = computed(() => [
  {
    title: t('task.workload'),
    dataIndex: 'name',
    hideTooltip: true,
    render: ({ name, appName, podUid, namespace, namespaceName }) => {
      const to = `/admin/vgpu/task/admin/detail?name=${name}&podUid=${podUid}`;
      const workloadName = appName || name || '--';
      const workloadNamespace = namespace || namespaceName || '--';
      return (
        <span style={{ display: 'inline-flex', alignItems: 'flex-start', gap: '10px' }}>
          <span class="task-name-icon-card vgpu-table-name-icon-card">
            <svg-icon icon="task-name" style={{ fontSize: '20px' }} />
          </span>
          <span class="task-name-text-wrap vgpu-table-name-text-wrap">
            <span style={{ display: 'inline-flex', flexDirection: 'column', minWidth: 0 }}>
              <text-plus text={workloadName} to={to} />
              <span class="task-namespace-text">{workloadNamespace}</span>
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
    title: 'GPUs',
    dataIndex: 'deviceIds',
    render: ({ deviceIds, allocatedCores, allocatedMem }) => {
      const ids = Array.isArray(deviceIds) ? deviceIds : [];
      const gpuCount = ids.length || '--';
      const cores = allocatedCores === 0 || allocatedCores
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
const { pagination, pagedTableData, syncTotalAndClamp, resetToFirstPage } = useLocalPagination(tableData);

const fetchTableData = async () => {
  tableLoading.value = true;
  try {
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
      },
    };
    const { items = [] } = await taskApi.getTaskListReq(payload);
    tableData.value = items;
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

onMounted(() => {
  fetchFilterOptions();
});

watch(
  () => [
    props.filters?.name,
    props.filters?.nodeName,
    props.filters?.nodeUid,
    props.filters?.status,
  ],
  () => {
    hasManualNodeScope.value = false;
    filters.name = props.filters?.name || '';
    filters.nodeName = props.filters?.nodeName;
    filters.status = props.filters?.status;
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
}

:deep(.task-name-icon-card) {
  user-select: none;
}

:deep(.task-name-text-wrap) {
  flex: 1;
}

:deep(.task-namespace-text) {
  color: #939ea9;
  font-size: 12px;
  line-height: 18px;
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
