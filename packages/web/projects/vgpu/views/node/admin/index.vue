<template>
    <list-header
      description="节点管理用于管理和监控计算节点的状态。它可以启用或禁用节点，查看节点上的物理GPU卡，以及监控节点上运行的所有任务。"
    />

    <preview-bar :handle-click=handleClick />

    <table-plus
      :api="nodeApi.getNodeList()"
      :columns="columns"
      :rowAction="rowAction"
      :searchSchema="searchSchema"
      :hasPagination="false"
      style="height: auto"
      hideTag
      ref="table"
      staticPage
    >
    </table-plus>

</template>

<script setup lang="jsx">
import nodeApi from '~/vgpu/api/node';
import searchSchema from '~/vgpu/views/node/admin/searchSchema';
import { useRouter } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import { roundToDecimal } from '@/utils';
import {ElMessage, ElMessageBox} from 'element-plus';
import { ref } from 'vue';

const router = useRouter();

const table = ref();

const handleClick = async (params) => {
  const name = params.data.name;
  const {list} = await nodeApi.getNodes({filters:{}})
  const node = list.find(node => node.name === name);
  if (node) {
    const uuid = node.uid;
    router.push(`/admin/vgpu/node/admin/${uuid}?nodeName=${name}`);
  } else {
    ElMessage.error('节点未找到');
  }
};

const columns = [
  {
    title: '节点名称',
    dataIndex: 'name',
    render: ({ uid, name }) => (
      <text-plus text={name} to={`/admin/vgpu/node/admin/${uid}?nodeName=${name}`} />
    ),
  },
  {
    title: '节点 IP',
    dataIndex: 'ip',
  },
  {
    title: '节点状态',
    dataIndex: 'isSchedulable',
    render: ({ isSchedulable, isExternal }) => (
        <el-tag disable-transitions type={isExternal ? 'warning' : (isSchedulable ? 'success' : 'danger')}>
          {isExternal ? '未纳管' : (isSchedulable ? '可调度' : '禁止调度')}
        </el-tag>
    )
    // filters: [
    //   {
    //     text: '可调度',
    //     value: 'true',
    //   },
    //   {
    //     text: '禁止调度',
    //     value: 'false',
    //   },
    // ],
  },
  {
    title: '显卡型号',
    dataIndex: 'type',
    // filters: (data) => {
    //   const r = data.reduce((all, item) => {
    //     return uniq([...all, ...item.type]);
    //   }, []);
    //
    //   return r.map((item) => ({ text: item, value: item }));
    // },
  },
  {
    title: '显卡数量',
    dataIndex: 'cardCnt',
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
      router.push(`/admin/vgpu/node/admin/${row.uid}?nodeName=${row.name}`);
    },
  },
  // {
  //   title: '禁用',
  //   hidden: (row) => !row.isSchedulable,
  //   onClick: async (row) => {
  //     ElMessageBox.confirm(`确认对该节点进行禁用操作？`, '操作确认', {
  //       confirmButtonText: '确定',
  //       cancelButtonText: '取消',
  //       type: 'warning',
  //     })
  //         .then(async () => {
  //           try {
  //             await nodeApi.stop(
  //                 {
  //                   nodeName: row.name,
  //                   switch: 'on'
  //                 }
  //             ).then(
  //                 () => {
  //                   setTimeout(() => {
  //                     ElMessage.success('节点禁用成功');
  //                     table.value.fetchData();
  //                   }, 500);
  //                 }
  //             )
  //           } catch (error) {
  //             ElMessage.error(error.message);
  //           }
  //         })
  //         .catch(() => {});
  //   },
  // },
  // {
  //   title: '开启',
  //   hidden: (row) => row.isSchedulable,
  //   disabled: (row) => row.isExternal,
  //   onClick: async (row) => {
  //     ElMessageBox.confirm(`确认对该节点进行开启调度操作？`, '操作确认', {
  //       confirmButtonText: '确定',
  //       cancelButtonText: '取消',
  //       type: 'warning',
  //     })
  //         .then(async () => {
  //           try {
  //             await nodeApi.stop(
  //                 {
  //                   nodeName: row.name,
  //                   switch: 'off'
  //                 }
  //             ).then(
  //                 () => {
  //                   setTimeout(() => {
  //                     ElMessage.success('节点开启调度成功');
  //                     table.value.fetchData();
  //                   }, 500);
  //                 }
  //             )
  //           } catch (error) {
  //             ElMessage.error(error.message);
  //           }
  //         })
  //         .catch(() => {});
  //   },
  // },
];
</script>

<style></style>
