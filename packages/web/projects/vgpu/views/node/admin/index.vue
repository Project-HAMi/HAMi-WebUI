<template>
    <list-header
      :description="$t('node.description')"
    />

    <preview-bar :handle-click=handleClick />

    <table-plus
      :key="locale"
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
import createSearchSchema from '~/vgpu/views/node/admin/searchSchema';
import { useRouter } from 'vue-router';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import { roundToDecimal } from '@/utils';
import {ElMessage, ElMessageBox} from 'element-plus';
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';

const router = useRouter();
const { t, locale } = useI18n();

const table = ref();

const searchSchema = computed(() => createSearchSchema(t));

const handleClick = async (params) => {
  const name = params.data.name;
  const {list} = await nodeApi.getNodes({filters:{}})
  const node = list.find(node => node.name === name);
  if (node) {
    const uuid = node.uid;
    router.push(`/admin/vgpu/node/admin/${uuid}?nodeName=${name}`);
  } else {
    ElMessage.error(t('node.nodeNotFound'));
  }
};

const columns = computed(() => [
  {
    title: t('node.name'),
    dataIndex: 'name',
    render: ({ uid, name }) => (
      <text-plus text={name} to={`/admin/vgpu/node/admin/${uid}?nodeName=${name}`} />
    ),
  },
  {
    title: t('node.ip'),
    dataIndex: 'ip',
  },
  {
    title: t('node.status'),
    dataIndex: 'isSchedulable',
    render: ({ isSchedulable, isExternal }) => (
        <el-tag disable-transitions type={isExternal ? 'warning' : (isSchedulable ? 'success' : 'danger')}>
          {isExternal ? t('node.unmanaged') : (isSchedulable ? t('dashboard.schedulable') : t('dashboard.unschedulable'))}
        </el-tag>
    )
  },
  {
    title: t('node.cardModel'),
    dataIndex: 'type',
  },
  {
    title: t('node.cardCount'),
    dataIndex: 'cardCnt',
  },
  {
    title: t('node.vgpu'),
    dataIndex: 'used',
    render: ({ vgpuTotal, vgpuUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : vgpuUsed}/{isExternal ? '--' : vgpuTotal}
    </span>
    ),
  },
  {
    title: t('node.computeAllocTotal'),
    dataIndex: 'used',
    minWidth: 100,
    render: ({ coreTotal, coreUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : coreUsed}/{coreTotal}
    </span>
    ),
  },
  {
    title: t('node.memoryAllocTotal'),
    dataIndex: 'w',
    minWidth: 100,
    render: ({ memoryTotal, memoryUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : roundToDecimal(memoryUsed / 1024, 1)}/
          {roundToDecimal(memoryTotal / 1024, 1)} GiB
    </span>
    ),
  },
]);

const rowAction = computed(() => [
  {
    title: t('node.viewDetails'),
    onClick: (row) => {
      router.push(`/admin/vgpu/node/admin/${row.uid}?nodeName=${row.name}`);
    },
  },
  // {
  //   title: t('node.disable'),
  //   hidden: (row) => !row.isSchedulable,
  //   onClick: async (row) => {
  //     ElMessageBox.confirm(t('node.confirmDisable'), t('node.operationConfirm'), {
  //       confirmButtonText: t('common.confirm'),
  //       cancelButtonText: t('common.cancel'),
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
  //                     ElMessage.success(t('node.disableSuccess'));
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
  //   title: t('node.enable'),
  //   hidden: (row) => row.isSchedulable,
  //   disabled: (row) => row.isExternal,
  //   onClick: async (row) => {
  //     ElMessageBox.confirm(t('node.confirmEnable'), t('node.operationConfirm'), {
  //       confirmButtonText: t('common.confirm'),
  //       cancelButtonText: t('common.cancel'),
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
  //                     ElMessage.success(t('node.enableSuccess'));
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
]);
</script>

<style></style>
