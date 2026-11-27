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
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import apiClient from '@/api/client.mjs';
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

const WORKLOAD_INVENTORY_PAGE_SIZE = 100;
const WORKLOAD_INVENTORY_MAX_PAGES = 10;

const router = useRouter();
const { t } = useI18n();
const taskCountQueries = buildTaskCountQueries();
const taskAllocationTopQueries = buildTaskAllocationTopQueries();

// Rankings cover the cluster, so their names must not depend on table filters.
const workloadInventory = ref([]);
let inventoryController = null;

const loadWorkloadInventory = async () => {
  inventoryController?.abort();
  const controller = new AbortController();
  inventoryController = controller;
  try {
    const items = [];
    for (let page = 1; page <= WORKLOAD_INVENTORY_MAX_PAGES; page += 1) {
      const result = await apiClient.listContainers(
        { page, pageSize: WORKLOAD_INVENTORY_PAGE_SIZE },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      const pageItems = Array.isArray(result?.items) ? result.items : [];
      items.push(...pageItems);
      const total = Number(result?.total ?? items.length);
      if (pageItems.length === 0 || items.length >= total) break;
    }
    workloadInventory.value = items;
  } catch {
    if (!controller.signal.aborted) workloadInventory.value = [];
  } finally {
    if (inventoryController === controller) inventoryController = null;
  }
};

onMounted(() => {
  loadWorkloadInventory();
});
onBeforeUnmount(() => {
  inventoryController?.abort();
  inventoryController = null;
});

const workloadIdentityIndex = computed(() => buildWorkloadIdentityIndex(workloadInventory.value));

const handleChartClick = async (params) => {
  const name = params.data.name;
  const activeTabKey = params.tabActive;
  if (activeTabKey === 'node') {
    try {
      const { list = [] } = await apiClient.listNodes();
      const node = list.find((item) => item.name === name);
      if (node) {
        const uuid = node.uid;
        router.push(`/nodes/${uuid}?nodeName=${name}`);
      } else {
        MessagePlugin.error(t('node.nodeNotFound'));
      }
    } catch {
      MessagePlugin.error(t('node.nodeNotFound'));
    }
  } else if (activeTabKey === 'device_uuid') {
    router.push({
      path: `/accelerators/${name}`,
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
