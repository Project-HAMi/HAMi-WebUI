<template>
  <div>
    <back-header> {{ $t('card.detail.title') }} : {{ detail.uuid }} </back-header>
    <block-box class="node-block">
      <div class="card-detail" :class="{ 'is-en': locale.startsWith('en') }">
        <div class="card-detail-left">
          <div class="title">{{ $t('card.detail.detailInfo') }}</div>
          <ul class="card-detail-info">
            <li v-for="{ label, value, render } in columns" :key="label">
              <span class="label">{{ label }}：</span>
              <span v-if="render" class="value">
                <component :is="render(detail)" />
              </span>
              <span v-else class="value">{{ detail[value] }}</span>
            </li>
          </ul>
        </div>
      </div>
    </block-box>

    <block-box :title="$t('card.detail.resourceOverview')">
      <ul class="resource-overview-cards">
        <li class="resource-overview-card">
          <div class="resource-card">
            <div class="resource-card-header">
              <div class="resource-card-icon">
                <svg-icon icon="vgpu-resource" />
              </div>
              <div class="resource-card-header-info">
                <div class="resource-card-value resource-card-value--compute">
                  {{ computeTotalText }}
                </div>
                <div class="resource-card-sub-title">
                  {{ $t('dashboard.computePowerTotal') }}
                </div>
              </div>
            </div>

            <div class="resource-card-footer">
              <div class="resource-card-rate-wrap">
                <div class="resource-card-footer-item">
                  <div class="resource-card-footer-title">
                    {{ $t('dashboard.allocated') }} / {{ $t('dashboard.allocRateLegend') }}
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric resource-card-footer-metric--allocated">{{ computeAllocUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ computeAllocPercentText }}</span>
                    <t-progress
                      v-if="computeAllocPercentProgress !== undefined"
                      theme="circle"
                      :percentage="computeAllocPercentProgressRounded"
                      size="24"
                      :color="getResourceColor(computeAllocPercentProgress)"
                      :label="false"
                    />
                  </div>
                </div>
              </div>

              <div class="resource-card-rate-wrap">
                <div class="resource-card-footer-item">
                  <div class="resource-card-footer-title">
                    {{ $t('dashboard.used') }} / {{ $t('dashboard.usageRateLegend') }}
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric">{{ computeUsageUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ computeUsagePercentText }}</span>
                    <t-progress
                      v-if="computeUsagePercentProgress !== undefined"
                      theme="circle"
                      :percentage="computeUsagePercentProgressRounded"
                      size="24"
                      :color="getResourceColor(computeUsagePercentProgress)"
                      :label="false"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </li>

        <li class="resource-overview-card">
          <div class="resource-card">
            <div class="resource-card-header">
              <div class="resource-card-icon">
                <svg-icon icon="vgpu-resource" />
              </div>
              <div class="resource-card-header-info">
                <div class="resource-card-value resource-card-value--compute">{{ memoryTotalText }}</div>
                <div class="resource-card-sub-title">
                  {{ $t('dashboard.memoryTotal') }}
                </div>
              </div>
            </div>

            <div class="resource-card-footer">
              <div class="resource-card-rate-wrap">
                <div class="resource-card-footer-item">
                  <div class="resource-card-footer-title">
                    {{ $t('dashboard.allocated') }} / {{ $t('dashboard.allocRateLegend') }}
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric resource-card-footer-metric--allocated">{{ memoryAllocUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ memoryAllocPercentText }}</span>
                    <t-progress
                      v-if="memoryAllocPercentProgress !== undefined"
                      theme="circle"
                      :percentage="memoryAllocPercentProgressRounded"
                      size="24"
                      :color="getResourceColor(memoryAllocPercentProgress)"
                      :label="false"
                    />
                  </div>
                </div>
              </div>

              <div class="resource-card-rate-wrap">
                <div class="resource-card-footer-item">
                  <div class="resource-card-footer-title">
                    {{ $t('dashboard.used') }} / {{ $t('dashboard.usageRateLegend') }}
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric">{{ memoryUsageUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ memoryUsagePercentText }}</span>
                    <t-progress
                      v-if="memoryUsagePercentProgress !== undefined"
                      theme="circle"
                      :percentage="memoryUsagePercentProgressRounded"
                      size="24"
                      :color="getResourceColor(memoryUsagePercentProgress)"
                      :label="false"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </li>
      </ul>
    </block-box>

    <trend-time-filter v-model="times" />
    <div class="line-box">
      <block-box :title="$t('dashboard.gpuComputeAllocUsageTrend')">
        <div class="trend-chart">
          <echarts-plus
            :options="
              getRangeOptions([
                {
                  name: t('dashboard.allocRateLegend'),
                  data: gaugeConfig[0]?.data,
                  itemStyle: {
                    borderColor: 'rgb(84, 112, 198)',
                  },
                  lineStyle: {
                    color: 'rgb(84, 112, 198)',
                  },
                },
                {
                  name: t('dashboard.usageRateLegend'),
                  data: gaugeConfig[2]?.data,
                  itemStyle: {
                    borderColor: 'rgb(145, 204, 117)',
                  },
                  lineStyle: {
                    color: 'rgb(145, 204, 117)',
                  },
                },
              ])
            "
          />
        </div>
      </block-box>
      <block-box :title="$t('dashboard.gpuMemAllocUsageTrend')">
        <div class="trend-chart">
          <echarts-plus
            :options="
              getRangeOptions([
                {
                  name: t('dashboard.allocRateLegend'),
                  data: gaugeConfig[1]?.data,
                  itemStyle: {
                    borderColor: 'rgb(84, 112, 198)',
                  },
                  lineStyle: {
                    color: 'rgb(84, 112, 198)',
                  },
                },
                {
                  name: t('dashboard.usageRateLegend'),
                  data: gaugeConfig[3]?.data,
                  itemStyle: {
                    borderColor: 'rgb(145, 204, 117)',
                  },
                  lineStyle: {
                    color: 'rgb(145, 204, 117)',
                  },
                },
              ])
            "
          />
        </div>
      </block-box>

      <block-box :title="title" v-for="{ title, data, unit } in lineToolsView" :key="title">
        <div class="trend-chart">
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
import { useRoute, useRouter } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { onMounted, ref, watch, computed } from 'vue';
import TaskList from '~/vgpu/views/task/admin/index.vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import EchartsPlus from '@/components/Echarts-plus.vue';
import cardApi from '~/vgpu/api/card';
import nodeApi from '~/vgpu/api/node';
import { timeParse, calculatePrometheusStep, roundToDecimal, getResourceColor } from '@/utils';
import { getLineOptions as getLineOptions2 } from '~/vgpu/components/config';
import { getRangeOptions } from '../../monitor/overview/getOptions';
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
    label: t('card.status'),
    value: 'health',
    render: ({ health, isExternal }) => {
      if (detail.value && detail.value.health !== undefined) {
        const icon = isExternal
          ? 'status-unmanaged'
          : (health ? 'status-schedulable' : 'status-unschedulable');
        const text = isExternal
          ? t('card.unknown')
          : (health ? t('card.normal') : t('card.abnormal'));
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
          </span>
        );
      } else {
        return <el-tag disable-transitions size="small" type="info">{t('card.detail.loading')}</el-tag>;
      }
    },
  },
  {
    label: t('card.uuid'),
    value: 'uuid',
    render: ({ uuid }) => (
      <ellipsis-text text={uuid || '--'} mode="middle" tooltip="always" />
    ),
  },
  {
    label: t('card.node'),
    value: 'nodeName',
    render: ({ nodeName }) => (
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
    ),
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
        <t-tag theme="default" variant="light">
          {type?.split('-')[0] === "NVIDIA" ? mode : 'default'}
        </t-tag>
    )
  },
  {
    label: t('card.detail.gpuPower'),
    value: 'gpuPowerTrend',
    render: () => {
      const v = lineTools.value[0]?.percent;
      const unit = lineTools.value[0]?.gaugeUnit || 'W';
      if (v === undefined || v === null) return <span class="detail-trend-value">--</span>;
      return (
        <span class="detail-trend-value">
          {Number(v).toFixed(1)} {unit}
        </span>
      );
    },
  },
  {
    label: t('card.detail.gpuTemperature'),
    value: 'gpuTemperatureTrend',
    render: () => {
      const v = lineTools.value[1]?.percent;
      const unit = lineTools.value[1]?.gaugeUnit || '℃';
      if (v === undefined || v === null) return <span class="detail-trend-value">--</span>;
      return (
        <span class="detail-trend-value">
          {Number(v).toFixed(1)} {unit}
        </span>
      );
    },
  },
]);

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

const gaugeData = useInstantVector(
  _gaugeConfigBase.map(item => ({ ...item, title: t(item.titleKey) })),
  (query) => query.replaceAll(`$deviceuuid`, route.params.uuid),
  times,
);

const gaugeConfig = computed(() =>
  gaugeData.value.map((item) => ({
    ...item,
    title: item.titleKey ? t(item.titleKey) : item.title,
  })),
);

const computeTotalText = computed(() => {
  const total = Number(gaugeConfig.value?.[0]?.total);
  return Number.isFinite(total) ? `${roundToDecimal(total, 1)}` : '--';
});

const memoryTotalText = computed(() => {
  const total = Number(gaugeConfig.value?.[1]?.total);
  return Number.isFinite(total) ? `${roundToDecimal(total, 1)} GiB` : '--';
});

const formatUsedValue = (v, unit) => {
  const n = Number(v);
  if (!Number.isFinite(n)) return '--';
  const text = `${roundToDecimal(n, 1)}`;
  return unit && String(unit).trim() ? `${text} ${unit}` : text;
};

const computeAllocUsedText = computed(() => (detail.value?.isExternal ? '--' : formatUsedValue(gaugeConfig.value?.[0]?.used, gaugeConfig.value?.[0]?.unit)));
const computeUsageUsedText = computed(() => formatUsedValue(gaugeConfig.value?.[2]?.used, gaugeConfig.value?.[2]?.unit));
const memoryAllocUsedText = computed(() => (detail.value?.isExternal ? '--' : formatUsedValue(gaugeConfig.value?.[1]?.used, gaugeConfig.value?.[1]?.unit)));
const memoryUsageUsedText = computed(() => formatUsedValue(gaugeConfig.value?.[3]?.used, gaugeConfig.value?.[3]?.unit));

const computeAllocPercentRaw = computed(() => {
  if (detail.value?.isExternal) return undefined;
  const p = Number(gaugeConfig.value?.[0]?.percent);
  return Number.isFinite(p) ? p : undefined;
});
const memoryAllocPercentRaw = computed(() => {
  if (detail.value?.isExternal) return undefined;
  const p = Number(gaugeConfig.value?.[1]?.percent);
  return Number.isFinite(p) ? p : undefined;
});
const computeUsagePercentRaw = computed(() => {
  const p = Number(gaugeConfig.value?.[2]?.percent);
  return Number.isFinite(p) ? p : undefined;
});
const memoryUsagePercentRaw = computed(() => {
  const p = Number(gaugeConfig.value?.[3]?.percent);
  return Number.isFinite(p) ? p : undefined;
});

const clampPercent = (v) => Math.max(0, Math.min(100, v));
const roundPercentForProgress = (p) => (p === undefined ? undefined : roundToDecimal(p, 2));

const computeAllocPercentProgress = computed(() => (computeAllocPercentRaw.value === undefined ? undefined : clampPercent(computeAllocPercentRaw.value)));
const computeUsagePercentProgress = computed(() => (computeUsagePercentRaw.value === undefined ? undefined : clampPercent(computeUsagePercentRaw.value)));
const memoryAllocPercentProgress = computed(() => (memoryAllocPercentRaw.value === undefined ? undefined : clampPercent(memoryAllocPercentRaw.value)));
const memoryUsagePercentProgress = computed(() => (memoryUsagePercentRaw.value === undefined ? undefined : clampPercent(memoryUsagePercentRaw.value)));

const computeAllocPercentProgressRounded = computed(() => roundPercentForProgress(computeAllocPercentProgress.value));
const computeUsagePercentProgressRounded = computed(() => roundPercentForProgress(computeUsagePercentProgress.value));
const memoryAllocPercentProgressRounded = computed(() => roundPercentForProgress(memoryAllocPercentProgress.value));
const memoryUsagePercentProgressRounded = computed(() => roundPercentForProgress(memoryUsagePercentProgress.value));

const computeAllocPercentText = computed(() => (computeAllocPercentRaw.value === undefined ? '--' : `${roundToDecimal(computeAllocPercentRaw.value, 2)}%`));
const computeUsagePercentText = computed(() => (computeUsagePercentRaw.value === undefined ? '--' : `${roundToDecimal(computeUsagePercentRaw.value, 2)}%`));
const memoryAllocPercentText = computed(() => (memoryAllocPercentRaw.value === undefined ? '--' : `${roundToDecimal(memoryAllocPercentRaw.value, 2)}%`));
const memoryUsagePercentText = computed(() => (memoryUsagePercentRaw.value === undefined ? '--' : `${roundToDecimal(memoryUsagePercentRaw.value, 2)}%`));

const lineTools = ref([
  {
    titleKey: 'card.detail.gpuPowerTrend',
    query: `avg by (device_no,driver_version) (hami_device_power{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: 'W',
    gaugeUnit: 'W',
    percent: undefined,
    total: 0,
    hideInfo: true,
    showProgress: false,
  },
  {
    titleKey: 'card.detail.gpuTemperatureTrend',
    query: `avg by (device_no,driver_version) (hami_device_temperature{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: '℃',
    gaugeUnit: '℃',
    percent: undefined,
    total: 0,
    hideInfo: true,
    showProgress: false,
  },
]);

const lineToolsView = computed(() =>
  lineTools.value.map((item) => ({
    ...item,
    title: t(item.titleKey),
  })),
);

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
        const first = res.data?.[0];
        const metric = first?.metric;
        if (metric) {
          const { device_no, driver_version } = metric;
          if (device_no && driver_version) {
            detail.value = { ...detail.value, device_no, driver_version };
          }
        }
        lineTools.value[index].data = first?.values || [];
      });

    cardApi
      .getInstantVector({
        query: item.query.replaceAll(`$deviceuuid`, route.params.uuid),
      })
      .then((res) => {
        lineTools.value[index].percent = res.data?.[0]?.value;
      });
  });
};

onMounted(async () => {
  const res = await cardApi.getCardDetail({ uid: route.params.uuid });
  detail.value = { ...detail.value, ...res };

  nodeUid.value = res?.nodeUid || res?.node_uid || '';
  if (!nodeUid.value && res?.nodeName) {
    try {
      const { list = [] } = await nodeApi.getNodes({ filters: {} });
      const node = list.find((item) => item?.name === res.nodeName);
      nodeUid.value = node?.uid || '';
    } catch {
      nodeUid.value = '';
    }
  }
});

watch(
  times,
  () => {
    fetchLineData();
  },
  { immediate: true },
);
</script>

<style scoped lang="scss">
.card-detail {
  display: flex;
  width: 100%;
  height: 100%;

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
  .card-detail-left {
    width: 100%;
  }

  .card-detail-info {
    width: 100%;
    column-gap: 15px;
    row-gap: 15px;
    display: grid;
    grid-template-columns: repeat(3, 1fr);

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

  }

  &.is-en {
    .card-detail-info {
      .label {
        width: 110px;
      }
    }
  }
}

.resource-overview-cards {
  margin: 15px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  gap: 15px;
}

.resource-overview-card {
  flex: 1;
}

.resource-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 15px 20px;
  background: #f5f7fa;
  border-radius: 8px;
}

.resource-card-header {
  display: flex;
  align-items: center;
  gap: 20px;
}

.resource-card-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0 4px 10px rgba(2, 5, 8, 0.06);
}

.resource-card-header-info {
  min-width: 0;
}

.resource-card-value {
  font-size: 20px;
  font-weight: 500;
  color: #1d2b3a;
  line-height: 28px;
}

.resource-card-value--compute {
  font-size: 16px;
  line-height: 22px;
}

.resource-card-sub-title {
  margin-top: 4px;
  font-size: 12px;
  color: #939ea9;
  line-height: 20px;
}

.resource-card-footer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 16px;
}

.resource-card-rate-wrap {
  width: 100%;
  background: #ffffff;
  border-radius: 8px;
  padding: 10px 12px;
  box-sizing: border-box;
}

.resource-card-footer-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.resource-card-footer-title {
  font-size: 12px;
  color: #939ea9;
  line-height: 20px;
}

.resource-card-footer-value {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.resource-card-footer-metric {
  font-size: 14px;
  font-weight: 500;
  color: #939ea9;
  white-space: nowrap;
}

.resource-card-footer-metric--allocated {
  color: #939ea9;
}

.resource-card-footer-sep {
  font-size: 12px;
  color: #b6c2cd;
}

.resource-card-footer-percent {
  font-size: 14px;
  font-weight: 500;
  color: #324558;
}

.trend-chart {
  height: 200px;
  margin-top: 15px;
}

.line-box {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;

  > .home-block {
    flex: 1 1 calc(50% - 10px);
    min-width: 0;
  }
}

.node-block {
  display: flex;
  flex-direction: column;
  .home-block-content {
    flex: 1;
  }
}

.node-jump-cell {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  min-width: 0;

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
}

.detail-trend-value {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
</style>
