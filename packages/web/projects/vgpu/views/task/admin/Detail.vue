<template>
  <back-header>
    {{ $t('task.detail.title') }} : {{ detail.name }}
  </back-header>

  <div class="task-top">
    <block-box>
      <div class="task-detail" :class="{ 'is-en': locale.startsWith('en') }">
        <div class="left">
          <div class="title">{{ $t('task.detail.detailInfo') }}</div>
          <ul class="node-detail-info">
            <li v-for="{ label, value, render } in columns" :key="label">
              <span class="label">{{ label }}：</span>
              <span v-if="render" class="value">
                <component :is="render(detail)" />
              </span>
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
      </div>
    </block-box>

    <block-box :title="$t('task.detail.resourceOverview')">
      <div class="task-gauges">
        <div v-for="item in gaugeConfig" :key="item.title">
          <Gauge v-bind="item" />
        </div>
      </div>
    </block-box>
  </div>

  <trend-time-filter
    v-if="detail.type && (detail.type.startsWith('NVIDIA') || detail.type.startsWith('MXC'))"
    v-model="times"
  />

  <div class="task-trend-row">
    <block-box v-for="{ title, data } in lineConfigView" :key="title" :title="title">
      <div class="trend-chart">
        <template v-if="detail.type && !detail.type.startsWith('NVIDIA') && !detail.type.startsWith('MXC')">
          <el-empty :description="$t('task.noMonitorSupport')" :image-size="60" />
        </template>
        <template v-else>
          <echarts-plus :options="getLineOptions({ data })" />
        </template>
      </div>
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import { useRoute, useRouter } from 'vue-router';
import { onMounted, ref, watch, computed } from 'vue';

import useInstantVector from '~/vgpu/hooks/useInstantVector';
import cardApi from '~/vgpu/api/card';
import nodeApi from '~/vgpu/api/node';
import { QuestionFilled } from '@element-plus/icons-vue';
import { ElPopover } from 'element-plus';
import { roundToDecimal, timeParse, calculatePrometheusStep } from '@/utils';
import taskApi from '~/vgpu/api/task';
import BlockBox from '@/components/BlockBox.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import { getLineOptions } from '~/vgpu/components/config';
import EchartsPlus from '@/components/Echarts-plus.vue';
import { useI18n } from 'vue-i18n';

const route = useRoute();
const router = useRouter();
const { t, locale } = useI18n();

const detail = ref({});
const nodeUid = ref('');

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
        closed: {
          text: t('task.statusCompleted'),
          icon: 'status-schedulable',
        },
        success: {
          text: t('task.statusRunning'),
          icon: 'status-schedulable',
        },
        unknown: {
          text: t('task.statusUnknown'),
          icon: 'status-unmanaged',
        },
        failed: {
          text: t('task.statusFailed'),
          icon: 'status-unschedulable',
        },
      };
      const { text, icon } = enums[status] || enums.unknown;
      return (
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '6px',
          }}
        >
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
          {(status === 'unknown' || status === 'failed') && (
              <ElPopover placement="top" trigger="hover" popper-style={{ width: '180px' }}>
                {{
                  reference: () => (
                    <el-icon color="#939EA9" size="14">
                      <QuestionFilled />
                    </el-icon>
                  ),
                  default: () => (
                      <span style={{ marginLeft: '5px', }}>
                      {t('task.checkCloudPlatform')}
                    </span>
                  ),
                }}
              </ElPopover>
          )}
        </span>
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
      return (
        <ellipsis-text
          text={deviceIds.join(', ')}
          mode="middle"
          tooltip="always"
        />
      );
    },
  },
  {
    label: t('task.node'),
    value: 'nodeName',
    render: ({ nodeName }) => {
      return (
        <div class="node-jump-cell">
          <ellipsis-text text={nodeName || '--'} mode="middle" tooltip="always" />
          <span
            class={['node-jump-icon', !nodeUid.value ? 'is-disabled' : ''].join(' ')}
            onClick={() => {
              if (!nodeUid.value) return;
              router.push(`/admin/vgpu/node/admin/${nodeUid.value}?nodeName=${nodeName || ''}`);
            }}
          >
            <svg-icon icon="jump" />
          </span>
        </div>
      );
    },
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
    render: ({ appName }) => (
      <ellipsis-text text={appName || '--'} mode="middle" tooltip="always" />
    ),
  },
  {
    label: t('task.createTime'),
    value: 'createTime',
    render: ({ createTime }) => <span>{timeParse(createTime)}</span>,
  },
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

const gaugeData = useInstantVector(
  _gaugeConfigBase.map(item => ({ ...item, title: t(item.titleKey) })),
  (query) =>
    query
      .replaceAll(`$container`, detail.value.name)
      .replaceAll(`$namespace`, detail.value.namespace)
      .replaceAll(`$pod`, detail.value.appName),
);

const gaugeConfig = computed(() =>
  gaugeData.value.map((item) => ({
    ...item,
    title: item.titleKey ? t(item.titleKey) : item.title,
  })),
);

const lineConfig = ref([
  {
    titleKey: 'task.computeUsageTrend',
    query: `avg(sum(hami_container_core_util{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))`,
    data: [],
  },
  {
    titleKey: 'task.memUsageTrend',
    query: `avg(sum(hami_container_memory_util{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (instance))`,
    data: [],
  },
]);

const lineConfigView = computed(() =>
  lineConfig.value.map((item) => ({
    ...item,
    title: t(item.titleKey),
  })),
);

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

  const initialNodeUid = detail.value.nodeUid || detail.value.node_uid || '';
  nodeUid.value = initialNodeUid;

  const nodeName = detail.value.nodeName || detail.value.node_name || '';
  if (!nodeUid.value && nodeName) {
    try {
      const { list = [] } = await nodeApi.getNodes({ filters: {} });
      const node = list.find((item) => item?.name === nodeName);
      nodeUid.value = node?.uid || '';
    } catch {
      nodeUid.value = '';
    }
  }
});
</script>

<style scoped lang="scss">
.task-top {
  display: flex;
  gap: 16px;

  > .home-block {
    flex: 7;
  }

  > .home-block:last-child {
    flex: 3;
  }
}

.task-detail {
  width: 100%;

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .title {
    color: #1d2b3a;
    font-family: 'PingFang SC';
    font-size: 16px;
    font-style: normal;
    font-weight: 500;
    margin-bottom: 20px;
  }

  .node-detail-info {
    width: 100%;
    column-gap: 15px;
    row-gap: 15px;
    display: grid;
    grid-template-columns: 1fr 1fr;

    li {
      display: flex;
      align-items: center;
      min-width: 0;
    }

    .label {
      display: inline-block;
      width: 80px;
      height: 20px;
      color: #939ea9;
      font-size: 12px;
      line-height: 20px;
      flex-shrink: 0;
    }

    .value {
      color: #324558;
      font-size: 14px;
      line-height: 22px;
      display: inline-flex;
      align-items: center;
      min-width: 0;
    }

    .cp {
      display: flex;
      gap: 25px;
    }
  }

  &.is-en {
    .node-detail-info {
      .label {
        width: 100px;
      }
    }
  }
}

.task-gauges {
  margin-top: 15px;
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.task-trend-row {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;

  > .home-block {
    flex: 1 1 calc(50% - 10px);
    min-width: 0;
  }
}

.trend-chart {
  height: 200px;
  margin-top: 15px;
}

.node-jump-cell {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  min-width: 0;
}

.node-jump-icon {
  flex: none;
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  border-radius: 4px;
  user-select: none;
}

.node-jump-icon.is-disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

</style>
