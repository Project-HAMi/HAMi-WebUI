<template>
  <list-header
    v-if="!hideTitle"
    :description="$t('task.description')"
  />

  <Top v-if="!hideTitle" />

  <table-plus
    :key="locale"
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
import { reactive, ref, computed } from 'vue';
import editSchema from './editSchema';
import { mapValues, isNumber, pick } from 'lodash';
import { useRouter } from 'vue-router';
import createSearchSchema from './searchSchema';
import Top from './top.vue';
import { useI18n } from 'vue-i18n';

const props = defineProps(['hideTitle', 'filters', 'style']);

const table = ref();

const router = useRouter();
const { t, locale } = useI18n();

const state = reactive({
  visible: false,
  schema: {},
  formValues: {},
  title: '',
  ok: () => {},
});

const searchSchema = computed(() => createSearchSchema(t));

const columns = computed(() => [
  {
    title: t('task.name'),
    dataIndex: 'name',
    render: ({ name,podUid }) => (
        <text-plus text={name} to={`/admin/vgpu/task/admin/detail?name=${name}&podUid=${podUid}`} />
    ),
  },
  {
    title: t('task.status'),
    dataIndex: 'status',
    render: ({ status, deviceIds }) => {
      const enums = {
        closed: { text: t('task.statusCompleted'), color: '#999' },
        success: { text: t('task.statusRunning'), color: '#2563eb' },
        unknown: { text: t('task.statusUnknown'), color: '#FACC15' },
        failed: { text: t('task.statusFailed'), color: '#EF4444' },
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
                      {t('task.checkCloudPlatform')}
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
    title: t('task.node'),
    dataIndex: 'nodeName',
  },
  {
    title: t('task.allocatedVgpu'),
    dataIndex: 'deviceIds',
    render: ({ deviceIds }) => {
      return (
        <ElPopover trigger="hover" popper-style={{ width: '350px' }}>
          {{
            reference: () => <el-tag disable-transitions>{deviceIds.length} {t('task.count')}</el-tag>,
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
    title: t('task.allocatedCompute'),
    dataIndex: 'allocatedCores',
    render: ({ allocatedCores }) => `${allocatedCores} `,
  },
  {
    title: t('task.allocatedMemory'),
    dataIndex: 'allocatedMem',
    render: ({ allocatedMem }) =>
      `${roundToDecimal(allocatedMem / 1024, 1)} GiB`,
  },

  {
    title: t('task.startTime'),
    dataIndex: 'createTime',
    render: ({ createTime }) => timeParse(createTime),
  },

]);

const rowAction = computed(() => [
  {
    title: t('task.viewDetails'),
    onClick: (row) => {
      router.push({
        path: '/admin/vgpu/task/admin/detail',
        query: pick(row, ['name', 'podUid']),
      });
    },
  },
]);
</script>

<style lang="scss"></style>
