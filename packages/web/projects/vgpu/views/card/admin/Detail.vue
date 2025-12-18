<template>
  <div>
    <back-header> {{ $t('card.detail.title') }} > {{ detail.uuid }} </back-header>
    <block-box class="node-block">
      <div class="card-detail">
        <div class="card-detail-left">
          <div class="title">{{ $t('card.detail.detailInfo') }}</div>
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
            <el-empty :description="$t('card.detail.noAllocData')" :image-size="90" />
          </template>
        </li>
        <li v-for="(item, index) in lineTools" :key="index">
          <Gauge v-bind="item" />
        </li>
      </ul>
    </block-box>


    <div class="line-box">
      <block-box :title="$t('card.detail.resourceAllocTrend')">
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
      <block-box :title="$t('card.detail.resourceUsageTrend')">
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

    <block-box :title="$t('card.detail.taskList')">
      <template v-if="detail.isExternal">
        <el-alert :title="$t('card.detail.unmanagedNoTask')" show-icon type="warning" :closable="false" />
        <el-empty :description="$t('card.detail.noTaskData')" :image-size="100" />
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
import { onMounted, ref, watch, computed } from 'vue';
import TaskList from '~/vgpu/views/task/admin/index.vue';
import {ElPopover} from 'element-plus';
import Gauge from '~/vgpu/components/gauge.vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import { QuestionFilled } from '@element-plus/icons-vue';
import EchartsPlus from '@/components/Echarts-plus.vue';
import cardApi from '~/vgpu/api/card';
import { timeParse, calculatePrometheusStep } from '@/utils';
import { getLineOptions } from '~/vgpu/views/monitor/overview/getOptions';
import { getLineOptions as getLineOptions2 } from '~/vgpu/components/config';
import TimeSelect from '~/vgpu/components/timeSelect.vue';
import { getRangeOptions } from './getOptions';
import { useI18n } from 'vue-i18n';

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
const { t } = useI18n();

const detail = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const columns = computed(() => [
  {
    label: t('card.status'),
    value: 'health',
    render: ({ health, isExternal }) => {
      if (detail.value && detail.value.health !== undefined) {
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
                {isExternal ? t('card.unmanaged') : (health ? t('card.healthy') : t('card.hardwareError'))}
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
                  {t('card.detail.checkHardware')}
                </span>
                      ),
                    }}
                  </ElPopover>
              )}
            </div>
        );
      } else {
        return <el-tag disable-transitions size="small" type="info">{t('card.detail.loading')}</el-tag>;
      }
    },
  },
  {
    label: t('card.detail.cardId'),
    value: 'uuid',
    render: ({ uuid }) => <text-plus text={uuid} copy />,
  },
  {
    label: t('card.node'),
    value: 'nodeName',
  },
  {
    label: t('card.model'),
    value: 'type',
  },
  {
    label: t('card.detail.deviceNo'),
    value: 'device_no',
  },
  {
    label: t('card.detail.driverVersion'),
    value: 'driver_version',
  },
  {
    label: t('card.mode'),
    value: 'mode',
    render: ({ mode, type }) => (
        <el-tag disable-transitions>
          {type?.split('-')[0] === "NVIDIA" ? mode : 'default'}
        </el-tag>
    )
  }
]);

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

const _gaugeConfigBase = [
  {
    titleKey: 'dashboard.computeAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vcore_allocated{deviceuuid=~"$deviceuuid"}) by (instance))`,
    totalQuery: `avg(sum(hami_core_size{deviceuuid=~"$deviceuuid"}) by (instance))`,
    percentQuery: `avg(sum(hami_container_vcore_allocated{deviceuuid=~"$deviceuuid"}) by (instance))/avg(sum(hami_core_size{deviceuuid=~"$deviceuuid"}) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vmemory_allocated{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024`,
    percentQuery: `(avg(sum(hami_container_vmemory_allocated{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024 )/(avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024) *100 `,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
  {
    titleKey: 'dashboard.computeUsageRate',
    percent: 0,
    query: `avg(sum(hami_core_util{deviceuuid=~"$deviceuuid"}) by (instance))`,
    percentQuery: `avg(sum(hami_core_util_avg{deviceuuid=~"$deviceuuid"}) by (instance))`,
    total: 100,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memUsageRate',
    percent: 0,
    query: `avg(sum(hami_memory_used{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance))/1024`,
    percentQuery: `(avg(sum(hami_memory_used{deviceuuid=~"$deviceuuid"}) by (instance)) / 1024)/(avg(sum(hami_memory_size{deviceuuid=~"$deviceuuid"}) by (instance))/1024)*100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
];

const gaugeConfig = useInstantVector(
  _gaugeConfigBase.map(item => ({...item, title: t(item.titleKey)})),
  (query) => query.replaceAll(`$deviceuuid`, route.params.uuid),
  times,
);

const lineTools = ref([
  {
    title: t('card.detail.gpuPower'),
    query: `avg by (device_no,driver_version) (hami_device_power{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: 'W',
    gaugeUnit: 'W',
    percent: 0,
    total: 0,
    hideInfo: true,
  },
  {
    title: t('card.detail.gpuTemperature'),
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
          step: calculatePrometheusStep(times.value[0], times.value[1]),
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
