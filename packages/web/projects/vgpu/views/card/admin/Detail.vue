<template>
  <div>
    <back-header> 显卡管理 > {{ detail.uuid }} </back-header>
    <block-box class="node-block">
      <div class="card-detail">
        <div class="card-detail-left">
          <div class="title">详细信息</div>
          <ul class="card-detail-info">
            <li v-for="{ label, value, render } in columns" :key="label">
              <span class="label">{{ label }}</span>
              <component v-if="render" :is="render(detail)" />
              <span v-else class="value">{{ detail[value] }}</span>
            </li>
          </ul>
        </div>
      </div>
    </block-box>

    <block-box>
      <ul class="card-gauges">
        <li v-for="(item, index) in gaugeConfig" :key="index">
          <template v-if="!detail.isExternal || index >= 2">
            <Gauge v-bind="item" />
          </template>
          <template v-else-if="detail.isExternal && index < 2">
            <el-empty description="暂无资源分配数据" :image-size="90" />
          </template>
        </li>
        <li v-for="(item, index) in lineTools" :key="index">
          <Gauge v-bind="item" />
        </li>
      </ul>
    </block-box>


    <div class="line-box">
      <block-box title="资源分配趋势（%）">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus
              :options="
              getRangeOptions({
                core: gaugeConfig[0].data,
                memory: gaugeConfig[1].data,
              })
            "
          />
        </div>
      </block-box>
      <block-box title="资源使用趋势（%）">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus
            :options="
              getRangeOptions({
                core: gaugeConfig[2].data,
                memory: gaugeConfig[3].data,
              })
            "
          />
        </div>
      </block-box>

      <block-box :title="title" v-for="{ title, data, unit } in lineTools" :key="title">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus :options="getLineOptions2({ data, unit })" />
        </div>
      </block-box>
    </div>

    <block-box title="任务列表">
      <template v-if="detail.isExternal">
        <el-alert title="由于显卡未纳管，无法获取到任务数据" show-icon type="warning" :closable="false" />
        <el-empty description="暂无任务数据" :image-size="100" />
      </template>
      <template v-else>
        <TaskList :hideTitle="true" :filters="{ deviceId: detail.uuid }" />
      </template>
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import { useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { onMounted, ref, watch, defineProps } from 'vue';
import TaskList from '~/vgpu/views/task/admin/index.vue';
import {ElPopover} from 'element-plus';
import Gauge from '~/vgpu/components/gauge.vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import { QuestionFilled } from '@element-plus/icons-vue';
import EchartsPlus from '@/components/Echarts-plus.vue';
import cardApi from '~/vgpu/api/card';
import { timeParse } from '@/utils';
import { getLineOptions } from '~/vgpu/views/monitor/overview/getOptions';
import { getLineOptions as getLineOptions2 } from '~/vgpu/components/config';
import TimeSelect from '~/vgpu/components/timeSelect.vue';
import { getRangeOptions } from './getOptions';

const props = defineProps([
  'title',
  'detailColumns',
  'type',
  'detail',
  'name',
  'filters',
  'hideCp',
]);

const route = useRoute();

const detail = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const columns = [
  {
    label: '显卡状态',
    value: 'health',
    render: ({ health, isExternal }) => {
      if (detail.value && detail.value.health !== undefined) {
        const text = health ? '健康' : '硬件错误';
        const color = health ? '#2563eb' : '#EF4444';
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
              <el-tag disable-transitions type={isExternal ? 'warning' : (health ? 'success' : 'danger')}>
                {isExternal ? '未纳管' : (health ? '健康' : '硬件错误')}
              </el-tag>
              {!health && (
                  <ElPopover trigger="hover" popper-style={{ width: '190px' }}>
                    {{
                      reference: () => (
                          <el-icon color="#939EA9" size="14">
                            <QuestionFilled />
                          </el-icon>
                      ),
                      default: () => (
                          <span style={{ marginLeft: '5px' }}>
                  请排查该 GPU 硬件问题
                </span>
                      ),
                    }}
                  </ElPopover>
              )}
            </div>
        );
      } else {
        return <el-tag disable-transitions size="small" type="info">加载中...</el-tag>;
      }
    },
  },
  // {
  //   label: '显卡厂商',
  //   value: 'uuid',
  //   render: ({ type }) => <span>{type?.split('-')[0]}</span>,
  // },
  {
    label: '显卡 ID',
    value: 'uuid',
    render: ({ uuid }) => <text-plus text={uuid} copy />,
  },
  {
    label: '所属节点',
    value: 'nodeName',
  },
  {
    label: '显卡型号',
    value: 'type',
  },
  {
    label: '设备号',
    value: 'device_no',
  },
  {
    label: '驱动版本',
    value: 'driver_version',
  },
];

const cp = useInstantVector(
  [
    {
      label: 'vGPU超配',
      count: '0',
      query: `avg(sum(hami_vgpu_count{node=~"$node"}) by (instance))`,
    },
    {
      label: '算力超配',
      count: '0',
      query: `avg(sum(hami_vcore_scaling{node=~"$node"}) by (instance))`,
    },
    {
      label: '显存超配',
      count: '1.5',
      query: `avg(sum(hami_vmemory_scaling{node=~"$node"}) by (instance))`,
    },
  ],
  (query) => query.replaceAll('$node', props.detail.name),
);

const gaugeConfig = useInstantVector(
  [
    {
      title: '算力分配率',
      percent: 0,
      query: `avg(sum(hami_container_vcore_allocated{deviceuuid=~"$deviceuuid"}) by (instance))`,
      totalQuery: `avg(sum(hami_core_size{deviceuuid=~"$deviceuuid"}) by (instance))`,
      percentQuery: `avg(sum(hami_container_vcore_allocated{deviceuuid=~"$deviceuuid"}) by (instance))/avg(sum(hami_core_size{deviceuuid=~"$deviceuuid"}) by (instance)) *100`,
      total: 0,
      used: 0,
      unit: ' ',
    },
    {
      title: '显存分配率',
      percent: 0,
      query: `avg(sum(hami_container_vmemory_allocated{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024`,
      totalQuery: `avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024`,
      percentQuery: `(avg(sum(hami_container_vmemory_allocated{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024 )/(avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024) *100 `,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
    {
      title: '算力使用率',
      percent: 0,
      query: `avg(sum(hami_core_util{deviceuuid=~"$deviceuuid"}) by (instance))`,
      percentQuery: `avg(sum(hami_core_util_avg{deviceuuid=~"$deviceuuid"}) by (instance))`,
      total: 100,
      used: 0,
      unit: ' ',
    },
    {
      title: '显存使用率',
      percent: 0,
      query: `avg(sum(hami_memory_used{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024`,
      totalQuery: `avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance))/1024`,
      percentQuery: `(avg(sum(hami_memory_used{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024)/(avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance))/1024)*100`,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
  ],
  (query) => query.replaceAll(`$deviceuuid`, route.params.uuid),
  times,
);

const lineTools = ref([
  {
    title: 'GPU功率 (W)',
    query: `avg by (device_no,driver_version) (hami_device_power{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: 'W',
    gaugeUnit: 'W',
    percent: 0,
    total: 0,
    hideInfo: true,
  },
  {
    title: 'GPU 温度（℃）',
    query: `avg by (device_no,driver_version) (hami_device_temperature{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: '℃',
    gaugeUnit: '℃',
    percent: 0,
    total: 0,
    hideInfo: true,
  },
]);

const fetchLineData = async () => {
  lineTools.value.map((item, index) => {
    cardApi
      .getRangeVector({
        range: {
          start: timeParse(times.value[0]),
          end: timeParse(times.value[1]),
          step: '1m',
        },
        query: item.query.replaceAll(`$deviceuuid`, route.params.uuid),
      })
      .then((res) => {
        const { device_no, driver_version } = res.data[0].metric;
        if (device_no && driver_version) {
          detail.value = { ...detail.value, device_no, driver_version };
        }
        lineTools.value[index].data = res.data[0]?.values || [];
      });

    cardApi
      .getInstantVector({
        query: item.query.replaceAll(`$deviceuuid`, route.params.uuid),
      })
      .then((res) => {
        lineTools.value[index].percent = res.data[0]?.value || 0;
      });
  });
};

onMounted(async () => {
  const res = await cardApi.getCardDetail({ uid: route.params.uuid });
  detail.value = { ...detail.value, ...res };
});

watch(
  times,
  () => {
    fetchLineData();
  },
  { immediate: true },
);
</script>

<style lang="scss">
.card-detail {
  display: flex;
  height: 100%;
  gap: 50px;

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
  .card-detail-left {
    min-width: 1050px;
  }
  .card-detail-info {
    //display: flex;
    //flex-direction: column;
    gap: 15px;
    font-size: 12px;
    display: grid;
    grid-template-columns: 3fr 2fr;

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

.card-gauges {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  height: 200px;
  li {
    flex: 1;
  }
}

.line-box {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: 20px;
}

.node-block {
  display: flex;
  flex-direction: column;
  .home-block-content {
    flex: 1;
  }
}
</style>
