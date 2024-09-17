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

const router = useRouter();

const handleChartClick = async (params) => {
  const name = params.data.name;
  const activeTabKey = params.tabActive;
  if (activeTabKey === 'node') {
    const { list } = await nodeApi.getNodes({ filters: {} });
    const node = list.find(node => node.name === name);
    if (node) {
      const uuid = node.uid;
      router.push(`/admin/vgpu/node/admin/${uuid}?nodeName=${name}`);
    } else {
      ElMessage.error('节点未找到');
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

const topConfig = [
  {
    title: '任务数量分布 Top5',
    key: 'total',
    config: [
      {
        tab: '节点',
        key: 'node',
        nameKey: 'node',
        data: [],
        unit: ' ',
        query:
          'topk(5, count by (node) (sum by (container_pod_uuid, node) (hami_container_vcore_allocated)))',
      },
      {
        tab: '显卡',
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
    title: '任务资源申请 Top5',
    key: 'apply',
    config: [
      {
        tab: '算力',
        key: 'core',
        data: [],
        nameKey: 'container_pod_uuid',
        unit: ' ',
        query: 'topk(5, avg by (container_pod_uuid) (hami_container_vcore_allocated))',
      },
      {
        tab: '显存',
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
];
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
