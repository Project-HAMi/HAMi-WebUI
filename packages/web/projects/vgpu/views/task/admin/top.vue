<template>
  <div class="task-top-box">
    <TabTop class="item" v-for="item in topConfig" :key="item.key" v-bind="item" :onClick="handleChartClick" />
  </div>
</template>

<script setup>
import TabTop from '~/vgpu/components/TabTop.vue';
import { useRouter } from 'vue-router';
import nodeApi from '~/vgpu/api/node';
import { ElMessage } from 'element-plus';
import { useI18n } from 'vue-i18n';
import { computed } from 'vue';

const router = useRouter();
const { t } = useI18n();

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
  } else if (activeTabKey === 'deviceuuid') {
    router.push({
      path: `/admin/vgpu/card/admin/${name}`,
    });
  } else {
    const [containerName, podUid] = name.split(':');
    router.push({
      path: '/admin/vgpu/task/admin/detail',
      query: {
        name: containerName,
        podUid: podUid,
      },
    });
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
        query:
          'topk(5, count by (node) (sum by (container_pod_uuid, node) (hami_container_vcore_allocated)))',
      },
      {
        tab: t('dashboard.card'),
        key: 'deviceuuid',
        data: [],
        nameKey: 'deviceuuid',
        unit: ' ',
        query:
          'topk(5, count by (deviceuuid) (sum by (container_pod_uuid, deviceuuid) (hami_container_vcore_allocated)))',
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
        unit: ' ',
        query: 'topk(5, avg by (container_pod_uuid) (hami_container_vcore_allocated))',
      },
      {
        tab: t('dashboard.memory'),
        key: 'memory',
        data: [],
        unit: 'GiB',
        nameKey: 'container_pod_uuid',
        query:
          'topk(5, avg by (container_pod_uuid) (hami_container_vmemory_allocated))/1024',
      },
      {
        tab: 'vGPU',
        key: 'vgpu',
        data: [],
        nameKey: 'container_pod_uuid',
        unit: '个',
        query: 'topk(5, avg by (container_pod_uuid) (hami_container_vgpu_allocated))',
      },
    ],
  },
]);
</script>

<style scoped lang="scss">
.task-top-box {
  display: flex;
  gap: 25px;
  .item {
    flex: 1;
  }
}
</style>
