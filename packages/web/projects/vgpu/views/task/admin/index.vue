<template>
  <list-header
    v-if="!hideTitle"
    description="任务管理用于监控物理显卡的状态。它用于监控物理显卡的分配使用情况，以及查看物理显卡上运行的所有任务。"
  />

  <Top v-if="!hideTitle" />

  <table-plus
    :api="taskApi.getTaskList({ filters })"
    :columns="columns"
    :rowAction="rowAction"
    :searchSchema="searchSchema"
    ref="table"
    :style="style"
    hideTag
    staticPage
  >
  </table-plus>

  <form-plus-drawer
    v-model="state.visible"
    v-model:form="state.formValues"
    :schema="state.schema"
    :title="state.title"
    @ok="state.ok"
  />
</template>

<script setup lang="jsx">
import taskApi from '~/vgpu/api/task';
import {calculateDuration, roundToDecimal, timeParse} from '@/utils';
import { QuestionFilled } from '@element-plus/icons-vue';
import api from '~/vgpu/api/task';
import {ElMessage, ElMessageBox, ElPopover} from 'element-plus';
import { reactive, ref, defineProps } from 'vue';
import editSchema from './editSchema';
import { mapValues, isNumber, pick } from 'lodash';
import { useRouter } from 'vue-router';
import searchSchema from './searchSchema';
import Top from './top.vue';

const props = defineProps(['hideTitle', 'filters', 'style']);

const table = ref();

const router = useRouter();

const state = reactive({
  visible: false,
  schema: {},
  formValues: {},
  title: '',
  ok: () => {},
});

const columns = [
  {
    title: '任务名称',
    dataIndex: 'name',
    render: ({ name,podUid }) => (
        <text-plus text={name} to={`/admin/vgpu/task/admin/detail?name=${name}&podUid=${podUid}`} />
    ),
  },
  {
    title: '任务状态',
    dataIndex: 'status',
    render: ({ status, deviceIds }) => {
      const enums = {
        closed: { text: '已完成', color: '#999' },
        success: { text: '运行中', color: '#2563eb' },
        unknown: { text: '未知', color: '#FACC15' },
        failed: { text: '错误', color: '#EF4444' },
      };
      const { text, color } = enums[status];
      return (
        <div
          style={{
            color,
            position: 'relative',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '5px',
          }}
        >
          <div
            style={{
              height: '7px',
              width: '7px',
              borderRadius: '50%',
              backgroundColor: color,
              display: 'inline-block',
            }}
          ></div>{' '}
          {text}
          {(status === 'unknown' || status === 'failed') && (
              <ElPopover trigger="hover" popper-style={{ width: '180px' }}>
                {{
                  reference: () => <el-icon color="#939EA9" size="14"><QuestionFilled /></el-icon>,
                  default: () => (
                    <span style={{ marginLeft: '5px', }}>
                      请跳转云平台查看详情
                    </span>
                  ),
                }}
              </ElPopover>
          )}
        </div>
      );
    },
  },
  {
    title: '所属节点',
    dataIndex: 'nodeName',
  },
  {
    title: '分配 vGPU',
    dataIndex: 'deviceIds',
    render: ({ deviceIds }) => {
      return (
        <ElPopover trigger="hover" popper-style={{ width: '350px' }}>
          {{
            reference: () => <el-tag disable-transitions>{deviceIds.length} 个</el-tag>,
            default: () => (
              <div
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '10px',
                  width: '100%',
                  justifyContent: 'center',
                }}
              >
                {deviceIds.map((item) => (
                  <ElTag type="info">{item}</ElTag>
                ))}
              </div>
            ),
          }}
        </ElPopover>
      );
    },
  },
  {
    title: '分配算力',
    dataIndex: 'allocatedCores',
    render: ({ allocatedCores }) => `${allocatedCores} `,
  },
  {
    title: '分配显存',
    dataIndex: 'allocatedMem',
    render: ({ allocatedMem }) =>
      `${roundToDecimal(allocatedMem / 1024, 1)} GiB`,
  },

  {
    title: '任务创建时间',
    dataIndex: 'createTime',
    render: ({ createTime }) => timeParse(createTime),
  },

];

const rowAction = [
  {
    title: '查看详情',
    onClick: (row) => {
      router.push({
        path: '/admin/vgpu/task/admin/detail',
        query: pick(row, ['name', 'podUid']),
      });
    },
  },
];
</script>

<style lang="scss"></style>
