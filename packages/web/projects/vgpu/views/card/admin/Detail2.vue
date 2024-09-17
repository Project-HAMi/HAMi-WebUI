<template>
  <Detail
    title="显卡"
    :detailColumns="columns"
    type="deviceuuid"
    :detail="detail"
    :name="detail.uuid"
    :filters="{ deviceId: detail.uuid }"
    hideCp="true"
  />
</template>

<script setup lang="jsx">
import Detail from '~/vgpu/components/Detail.vue';
import { onMounted, ref } from 'vue';
import nodeApi from '~/vgpu/api/node';
import { useRoute } from 'vue-router';
import cardApi from '~/vgpu/api/card';

const detail = ref({});

const route = useRoute();

const columns = [
  // {
  //   label: '显卡厂商',
  //   value: 'uuid',
  //   render: ({ type }) => <span>{type?.split('-')[0]}</span>,
  // },
  {
    label: '显卡 ID',
    value: 'uuid',
  },
  {
    label: '所属节点',
    value: 'nodeName',
  },
  {
    label: '显卡型号',
    value: 'type',
  },
  // {
  //   label: 'vGPU(已分配/总量)',
  //   value: 'vgpu',
  //   render: ({ vgpuTotal, vgpuUsed }) => (
  //     <span>
  //       {vgpuUsed}/{vgpuTotal}
  //     </span>
  //   ),
  // },
  // {
  //   label: '算力(已分配/总量)',
  //   value: 'allocatedCores',
  //   render: ({ coreTotal, coreUsed }) => (
  //     <span>
  //       {coreUsed}/{coreTotal}
  //     </span>
  //   ),
  // },
  // {
  //   label: '显存(已分配/总量)',
  //   value: 'allocatedDevices',
  //   render: ({ memoryTotal, memoryUsed }) => (
  //     <span>
  //       {memoryUsed}/{memoryTotal} M
  //     </span>
  //   ),
  // },
  // {
  //   label: '显卡版本',
  //   value: 'DCGM_FI_DRIVER_VERSION',
  // },
  // {
  //   label: '设备号',
  //   value: 'device',
  // },
];

onMounted(async () => {
  detail.value = await cardApi.getCardDetail({ uid: route.params.uuid });
});
</script>
