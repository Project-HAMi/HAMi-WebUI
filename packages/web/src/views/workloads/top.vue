<template>
  <div class="task-top-box">
    <TabTop class="item" v-for="config in topConfig" :key="config.key" v-bind="config" :onClick="handleChartClick">
      <template v-if="config.key === 'apply'" #item-name="{ item }">
        <WorkloadRankingName :workload="item.workload" />
      </template>
    </TabTop>
  </div>
</template>

<script setup>
import TabTop from '@/components/TabTop.vue';
import { useRouter } from 'vue-router';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import apiClient from '@/api/client.mjs';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import WorkloadRankingName from './WorkloadRankingName.vue';
import { createWorkloadDetailLocation } from './workload-identity.mjs';

const router = useRouter();
const { t } = useI18n();

const rankings = ref(null);
const rankingStatus = ref(REQUEST_STATUS.LOADING);
let rankingsController = null;

const loadRankings = async () => {
  rankingsController?.abort();
  const controller = new AbortController();
  rankingsController = controller;
  rankingStatus.value = REQUEST_STATUS.LOADING;
  try {
    const reply = await apiClient.getContainerRankings({ signal: controller.signal });
    if (controller.signal.aborted) return;
    rankings.value = reply;
    rankingStatus.value = REQUEST_STATUS.READY;
  } catch {
    if (!controller.signal.aborted) rankingStatus.value = REQUEST_STATUS.ERROR;
  } finally {
    if (rankingsController === controller) rankingsController = null;
  }
};

onMounted(() => {
  loadRankings();
});
onBeforeUnmount(() => {
  rankingsController?.abort();
  rankingsController = null;
});

const toCountRows = (items = []) => items.map((item) => ({
  name: item.name || '-',
  nodeUid: item.nodeUid || '',
  value: Number(item.count) || 0,
}));

const toAllocationRows = (items = []) => items.map((item) => {
  const metricName = `${item.name || ''}:${item.podUid || ''}`;
  return {
    name: metricName,
    value: Number(item.value) || 0,
    workload: {
      metricName,
      identity: item.name && item.podUid ? { name: item.name, podUid: item.podUid } : null,
      appName: item.appName || '',
      namespace: item.namespace || '',
    },
  };
});

const handleChartClick = async (params) => {
  const name = params.data.name;
  const activeTabKey = params.tabActive;
  if (activeTabKey === 'node') {
    const uid = params.data.nodeUid;
    if (uid) {
      router.push({ name: 'node-detail', params: { uid }, query: { nodeName: name } });
      return;
    }
    try {
      const { list = [] } = await apiClient.listNodes();
      const node = list.find((item) => item.name === name);
      if (node) {
        router.push(`/nodes/${node.uid}?nodeName=${name}`);
      } else {
        MessagePlugin.error(t('node.nodeNotFound'));
      }
    } catch {
      MessagePlugin.error(t('node.nodeNotFound'));
    }
  } else if (activeTabKey === 'device_uuid') {
    const nodeUid = params.data.nodeUid;
    router.push({
      name: 'accelerator-detail',
      params: { uuid: name },
      ...(nodeUid ? { query: { nodeUid } } : {}),
    });
  } else {
    const location = createWorkloadDetailLocation(name);
    if (location) router.push(location);
  }
};

const topConfig = computed(() => {
  const current = rankings.value;
  const status = rankingStatus.value;
  return [
    {
      title: t('task.topCount'),
      key: 'total',
      config: [
        {
          tab: t('dashboard.node'),
          key: 'node',
          status,
          data: toCountRows(current?.byNode),
          unit: ' ',
        },
        {
          tab: t('dashboard.card'),
          key: 'device_uuid',
          status,
          data: toCountRows(current?.byDevice),
          unit: ' ',
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
          status,
          data: toAllocationRows(current?.byCompute),
          unit: t('dashboard.acceleratorEquivalentUnit'),
        },
        {
          tab: t('dashboard.memory'),
          key: 'memory',
          status,
          data: toAllocationRows(current?.byMemory),
          unit: 'GiB',
        },
        {
          tab: 'vGPU',
          key: 'vgpu',
          status,
          data: toAllocationRows(current?.byVgpu),
          unit: t('dashboard.vgpuSlotUnit'),
        },
      ],
    },
  ];
});
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
