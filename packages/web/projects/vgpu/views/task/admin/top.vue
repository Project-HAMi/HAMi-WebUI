<template>
  <div class="task-top-box">
    <TabTop class="item" v-for="config in topConfig" :key="config.key" v-bind="config" :onClick="handleChartClick">
      <template v-if="config.key === 'apply'" #item-name="{ item }">
        <WorkloadRankingName :workload="resolveWorkloadRankingIdentity(item.name, workloadIdentityIndex)" />
      </template>
    </TabTop>
  </div>
</template>

<script setup>
import TabTop from '~/vgpu/components/TabTop.vue';
import { useRouter } from 'vue-router';
import nodeApi from '~/vgpu/api/node';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { computed } from 'vue';
import taskApi from '~/vgpu/api/task';
import useFetchList from '@/hooks/useFetchList';
import WorkloadRankingName from './WorkloadRankingName.vue';
import {
  buildWorkloadIdentityIndex,
  createWorkloadDetailLocation,
  resolveWorkloadRankingIdentity,
} from './workload-identity.mjs';
import {
  buildTaskAllocationTopQueries,
  buildTaskCountQueries,
} from '~/vgpu/metrics/query-contract.mjs';

const router = useRouter();
const { t } = useI18n();
const taskCountQueries = buildTaskCountQueries();
const taskAllocationTopQueries = buildTaskAllocationTopQueries();

// Rankings cover the cluster, so their names must not depend on table filters.
const { data: workloadInventory } = useFetchList(
  () => taskApi.getTaskListReq({ filters: {} }),
  { path: 'items' },
);
const workloadIdentityIndex = computed(() => buildWorkloadIdentityIndex(workloadInventory.value));

const handleChartClick = async (params) => {
  const name = params.data.name;
  const activeTabKey = params.tabActive;
  if (activeTabKey === 'node') {
    const { list } = await nodeApi.getNodes({ filters: {} });
    const node = list.find((node) => node.name === name);
    if (node) {
      const uuid = node.uid;
      router.push(`/admin/vgpu/node/admin/${uuid}?nodeName=${name}`);
    } else {
      ElMessage.error(t('node.nodeNotFound'));
    }
  } else if (activeTabKey === 'device_uuid') {
    router.push({
      path: `/admin/vgpu/card/admin/${name}`,
    });
  } else {
    const location = createWorkloadDetailLocation(name);
    if (location) router.push(location);
  }
};

const topConfig = computed(() => [
  {
    title: t('task.topCount'),
    key: 'total',
    config: [
      {
        tab: t('dashboard.node'),
        key: 'node',
        nameKey: 'node',
        data: [],
        unit: ' ',
        query: taskCountQueries.byNode,
      },
      {
        tab: t('dashboard.card'),
        key: 'device_uuid',
        data: [],
        nameKey: 'device_uuid',
        unit: ' ',
        query: taskCountQueries.byDevice,
      },
    ],
  },
  {
    title: t('task.topApply'),
    key: 'apply',
    config: [
      {
        tab: t('dashboard.compute'),
        key: 'core',
        data: [],
        nameKey: 'container_pod_uuid',
        unit: t('dashboard.acceleratorEquivalentUnit'),
        query: taskAllocationTopQueries.compute,
      },
      {
        tab: t('dashboard.memory'),
        key: 'memory',
        data: [],
        unit: 'GiB',
        nameKey: 'container_pod_uuid',
        query: taskAllocationTopQueries.memory,
      },
      {
        tab: 'vGPU',
        key: 'vgpu',
        data: [],
        nameKey: 'container_pod_uuid',
        unit: t('dashboard.vgpuSlotUnit'),
        query: taskAllocationTopQueries.vgpu,
      },
    ],
  },
]);
</script>

<style scoped lang="scss">
.task-top-box {
  display: flex;
  gap: 16px;
  .item {
    flex: 1;
    min-width: 0;
  }
}
</style>
