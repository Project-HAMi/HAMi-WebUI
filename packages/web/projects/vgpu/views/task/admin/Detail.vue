<template>
  <back-header>
    {{ $t('task.detail.title') }} > {{ detail.name }}
  </back-header>

  <block-box>
    <div class="task-detail">
      <div class="left">
        <!--        <el-descriptions column="2" title="详细信息">-->
        <!--          <el-descriptions-item-->
        <!--            v-for="{ label, value, render } in columns"-->
        <!--            :label="label"-->
        <!--          >-->
        <!--            <component v-if="render" :is="render(detail)" />-->
        <!--            <span v-else class="value">{{ detail[value] || '&#45;&#45;' }}</span>-->
        <!--          </el-descriptions-item>-->
        <!--        </el-descriptions>-->
        <div class="title">{{ $t('task.detail.detailInfo') }}</div>
        <ul class="node-detail-info">
          <li v-for="{ label, value, render } in columns" :key="label">
            <span class="label">{{ label }}</span>
            <component v-if="render" :is="render(detail)" />
            <span v-else class="value">{{ detail[value] }}</span>
          </li>


          <li class="cp">
            <span v-for="{ label, count } in cp" :key="label">
              <span class="label">{{ label }}</span>

              <span class="value">{{ count }} {{ $t('task.times') }}</span>
            </span>
          </li>
        </ul>
      </div>
      <div class="right">
        <div v-for="item in gaugeConfig" :key="item.title">
          <Gauge v-bind="item" />
        </div>
      </div>
    </div>
  </block-box>

  <block-box v-for="{ title, data } in lineConfig" :key="title" :title="title">
    <template #extra v-if="detail.type && (detail.type.startsWith('NVIDIA') || detail.type.startsWith('MXC') || detail.type.startsWith('BI-') || detail.type.startsWith('MR-'))">
      <time-picker v-model="times" type="datetimerange" size="small" />
    </template>
    <div style="height: 200px">
      <template v-if="detail.type && !detail.type.startsWith('NVIDIA') && !detail.type.startsWith('MXC') && !detail.type.startsWith('BI-') && !detail.type.startsWith('MR-')">
        <el-empty :description="$t('task.noMonitorSupport')" :image-size="60" />
      </template>
      <template v-else>
        <echarts-plus :options="getLineOptions({ data })" />
      </template>
    </div>
  </block-box>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import {useRoute, useRouter} from 'vue-router';
import { onMounted, ref, watch, watchEffect, computed } from 'vue';

import useInstantVector from '~/vgpu/hooks/useInstantVector';
import cardApi from '~/vgpu/api/card';
import { QuestionFilled } from '@element-plus/icons-vue';
import { ElPopover } from 'element-plus';
import { roundToDecimal, timeParse, calculateDuration, calculatePrometheusStep } from '@/utils';
import taskApi from '~/vgpu/api/task';
import BlockBox from '@/components/BlockBox.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import { getLineOptions } from '~/vgpu/components/config';
import EchartsPlus from '@/components/Echarts-plus.vue';
import TimeSelect from '~/vgpu/components/timeSelect.vue';
import { useI18n } from 'vue-i18n';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const detail = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const columns = computed(() => [
  {
    label: t('task.status'),
    value: 'status',
    render: ({ status }) => {
      const enums = {
        closed: { text: t('task.statusCompleted'), color: '#999' },
        success: { text: t('task.statusRunning'), color: '#2563eb' },
        unknown: { text: t('task.statusUnknown'), color: '#FACC15' },
        failed: { text: t('task.statusFailed'), color: '#EF4444' },
      };
      const { text, color } = enums[status] || {};
      return (
        <div
          style={{
            color,
            position: 'relative',
            display: 'inline-flex',
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
              <ElPopover placement="top" trigger="hover" popper-style={{ width: '180px' }}>
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
    label: t('task.card'),
    value: 'deviceIds',
    render: ({ deviceIds }) => {
      if (!deviceIds || !Array.isArray(deviceIds) || deviceIds.length === 0) {
        return <span>--</span>;
      }

      const text = deviceIds.join(', ');
      const maxLength = 25;
      const isLongText = text.length > maxLength;
      const displayText = isLongText ? `${text.slice(0, maxLength)}...` : text;

      return isLongText ? (
          <el-tooltip content={text} placement="top">
            <span>{displayText}</span>
          </el-tooltip>
      ) : (
          <span>{displayText}</span>
      );
    },
  },
  {
    label: t('task.node'),
    value: 'nodeName',
    render: ({ nodeName }) => <text-plus text={nodeName} copy />,
  },
  {
    label: t('task.cardType'),
    value: 'type',
  },
  {
    label: t('task.allocatedCompute'),
    value: 'allocatedCores',
  },
  {
    label: t('task.allocatedMemory'),
    value: 'allocatedMem',
    render: ({ allocatedMem }) =>
      allocatedMem ? (
        <span>{roundToDecimal(allocatedMem / 1024, 1)} GiB</span>
      ) : (
        <span>--</span>
      ),
  },
  {
    label: t('task.appName'),
    value: 'appName',
  },
  {
    label: t('task.createTime'),
    value: 'createTime',
    render: ({ createTime }) => <span>{timeParse(createTime)}</span>,
  },
  // {
  //   label: '任务运行时长',
  //   value: 'createTime',
  //   render: ({ createTime, status }) =>
  //       status === 'success' ? <span>{calculateDuration(createTime)}</span> : null,
  // },
]);
const _gaugeConfigBase = [
  {
    titleKey: 'dashboard.computeUsageRate',
    percent: 0,
    query: `avg(sum(hami_container_core_used{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))`,
    totalQuery: `avg(sum(hami_container_vcore_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))`,
    percentQuery: `avg(sum(hami_container_core_used{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance)) / avg(sum(hami_container_vcore_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: '%',
    data: [],
  },
  {
    titleKey: 'dashboard.memUsageRate',
    percent: 0,
    query: `avg(sum(hami_container_memory_used{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))/ 1024`,
    totalQuery: `avg(sum(hami_container_vmemory_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))/1024`,
    percentQuery: `(avg(sum(hami_container_memory_used{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"})/ 1024)/(avg(sum(hami_container_vmemory_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))/1024) *100)`,
    total: 0,
    used: 0,
    unit: 'GiB',
    data: [],
  },
];

const gaugeConfig = useInstantVector(
  _gaugeConfigBase.map(item => ({...item, title: t(item.titleKey)})),
  (query) =>
    query
      .replaceAll(`$container`, detail.value.name)
      .replaceAll(`$namespace`, detail.value.namespace)
      .replaceAll(`$pod`, detail.value.appName),
);

const lineConfig = ref([
  {
    title: t('task.computeUsageTrend'),
    query: `avg(sum(hami_container_core_util{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))`,
    data: [],
  },
  {
    title: t('task.memUsageTrend'),
    query: `avg(sum(hami_container_memory_util{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))`,
    data: [],
  },
]);

const fetchLineData = async () => {

  lineConfig.value.map((item, index) =>
    cardApi
      .getRangeVector({
        range: {
          start: timeParse(times.value[0]),
          end: timeParse(times.value[1]),
          step: calculatePrometheusStep(times.value[0], times.value[1]),
        },
        query: item.query
          .replaceAll(`$container`, detail.value.name)
          .replaceAll(`$namespace`, detail.value.namespace)
          .replaceAll(`$pod`, detail.value.appName),
      })
      .then((res) => {
        lineConfig.value[index].data = res.data[0]?.values || [];
      }),
  );
};

watch(detail, async () => {
  fetchLineData();
});

watch(times, () => {
  fetchLineData();
});

onMounted(async () => {
  const { name, podUid } = route.query;
  detail.value = await taskApi.getTaskDetail({ name, podUid });
  const cards = await cardApi.getCardListReq({filters: {}});
  const foundCard = cards.list.find((item) => item.uuid === detail.value.deviceIds[0]);
  if (foundCard) {
    detail.value.type = foundCard.type;
  }

  // const start = new Date();
  // start.setTime(start.getTime() - 3600 * 1000 * 24 * 7);
  // const lineReqs = gaugeConfig.value.map((item) =>
  //   cardApi.getRangeVector({
  //     range: {
  //       start: timeParse(start),
  //       end: timeParse(new Date()),
  //       step: '1m',
  //     },
  //     query: item.query
  //       .replaceAll(`$container`, detail.value.name)
  //       .replaceAll(`$namespace`, detail.value.namespace)
  //       .replaceAll(`$pod`, detail.value.appName),
  //   }),
  // );
  //
  // const res = await Promise.all(lineReqs);
  //
  // gaugeConfig.value = gaugeConfig.value.map((item, index) => ({
  //   ...item,
  //   data: res[index].data[0]?.values || [],
  // }));
});
</script>

<style lang="scss">
.task-detail {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  .right {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
  }

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .title {
    color: #1d2b3a;
    font-family: 'PingFang SC';
    font-size: 14px;
    font-style: normal;
    font-weight: 500;
    //line-height: 20px;
    margin-bottom: 20px;
  }

  .node-detail-info {
    gap: 15px;
    font-size: 12px;
    display: grid;
    grid-template-columns: 1fr 1fr;

    .label {
      display: inline-block;
      width: 80px;
      height: 20px;
      color: #939ea9;
    }

    .cp {
      display: flex;
      gap: 25px;
    }
  }
}
</style>
