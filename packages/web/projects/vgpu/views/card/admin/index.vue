<template>
  <list-header
    v-if="!hideTitle"
    description="显卡管理用于监控物理显卡的状态。它用于监控物理显卡的分配使用情况，以及查看物理显卡上运行的所有任务。"
  />

  <preview-bar
    v-if="!hideTitle"
    title="显卡"
    type="deviceuuid"
    :handle-click="handleClick"
    :handle-pie-click="handlePieClick"
    :currentName="tableRef.currentParams?.filters?.type"
  />

  <table-plus
    :api="cardApi.getCardList({ filters })"
    :columns="columns"
    :rowAction="rowAction"
    :searchSchema="searchSchema"
    hideTag
    style="height: auto"
    ref="tableRef"
    staticPage
  >
  </table-plus>
</template>

<script setup lang="jsx">
import cardApi from '~/vgpu/api/card';
import { useRouter } from 'vue-router';
import searchSchema from '~/vgpu/views/card/admin/searchSchema';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import { defineProps, ref, watch } from 'vue';
import { roundToDecimal } from '@/utils';

const props = defineProps(['hideTitle', 'filters']);

const router = useRouter();

const tableRef = ref({});

const handleClick = (params) => {
  router.push({
    path: `/admin/vgpu/card/admin/${params.data.name}`,
  });
};

const columns = [
  {
    title: '显卡 ID',
    dataIndex: 'uuid',
    render: ({ uuid }) => (
      <text-plus text={uuid} to={`/admin/vgpu/card/admin/${uuid}`} />
    ),
  },
  {
    title: '显卡状态',
    dataIndex: 'health',
    render: ({ health, isExternal }) => (
        <el-tag disable-transitions type={isExternal ? 'warning' : (health ? 'success' : 'danger')}>
          {isExternal ? '未纳管' : (health ? '健康' : '硬件错误')}
        </el-tag>
    )
  },
  {
    title: '使用模式',
    dataIndex: 'mode',
    render: ({ mode, type }) => (
        <el-tag disable-transitions>
          {type?.split('-')[0] === "NVIDIA" ? mode : 'default'}
        </el-tag>
    )
  },
  {
    title: '所属节点',
    dataIndex: 'nodeName',
  },
  {
    title: '显卡型号',
    dataIndex: 'type',
  },
  {
    title: 'vGPU',
    dataIndex: 'used',
    render: ({ vgpuTotal, vgpuUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : vgpuUsed}/{isExternal ? '--' : vgpuTotal}
    </span>
    ),
  },
  {
    title: '算力(已分配/总量)',
    dataIndex: 'used',
    render: ({ coreTotal, coreUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : coreUsed}/{coreTotal}
    </span>
    ),
  },
  {
    title: '显存(已分配/总量)',
    dataIndex: 'w',
    render: ({ memoryTotal, memoryUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : roundToDecimal(memoryUsed / 1024, 1)}/
          {roundToDecimal(memoryTotal / 1024, 1)} GiB
    </span>
    ),
  },
];

const rowAction = [
  {
    title: '查看详情',
    onClick: (row) => {
      router.push({
        path: `/admin/vgpu/card/admin/${row.uuid}`,
      });
    },
  },
];

const PieRef = ref();

const handlePieClick = (params, echarts) => {
  PieRef.value = echarts;
  const name = params.data.name;
  if (tableRef.value.currentParams.filters.type === name) {
    echarts.dispatchAction({
      type: 'downplay',
      seriesIndex: 0,
    });
    return delete tableRef.value.currentParams.filters.type;
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
  tableRef.value.currentParams.filters.type = name;
};

watch(
  () => tableRef.value.currentParams?.filters?.type,
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
</script>

<style></style>
