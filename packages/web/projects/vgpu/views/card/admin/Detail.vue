<template>
  <div>
    <page-header
      :title="$t('card.detail.title')"
      :name="detail.uuid"
      :status="headerStatusDisplay.text"
      :status-icon="headerStatusDisplay.icon"
    />
    <block-box class="node-block">
      <div class="card-detail">
        <div class="card-detail-left">
          <div class="title">{{ $t('card.detail.detailInfo') }}</div>
          <div class="basic-info-row">
            <div class="basic-info-card">
              <div class="basic-info-title" @click="handleToNodeDetail">
                <span class="text">{{ detail.nodeName || '--' }}</span>
                <span class="basic-info-share">
                  <svg-icon icon="jump" />
                </span>
              </div>
              <div class="basic-info-subtitle">{{ $t('card.node') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                <svg-icon v-if="gpuTypeIcon && detail.type" :icon="gpuTypeIcon" class="gpu-type-icon" />
                {{ detail.type || '--' }}
              </div>
              <div class="basic-info-subtitle">{{ $t('card.model') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                {{ basicTemperatureText }}
              </div>
              <div class="basic-info-subtitle">{{ $t('card.detail.gpuTemperature') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                {{ basicPowerText }}
              </div>
              <div class="basic-info-subtitle">{{ $t('card.detail.gpuPower') }}</div>
            </div>
          </div>
        </div>
      </div>
    </block-box>

    <block-box class="resource-overview-block" :title="$t('card.detail.resourceOverview')">
      <ul class="resource-overview-cards">
        <li class="resource-overview-card">
          <div class="progress-wrapper">
            <workload-semi-progress :percent="workloadCountPercentProgress" />
            <div class="workload-progress-summary">
              <div class="workload-progress-value">
                <b>{{ workloadCountUsedText }}</b> / {{ workloadCountTotalText }}
              </div>
              <div class="workload-progress-subtitle">
                <span>{{ $t('card.detail.workloadCount') }}</span>
                <t-tooltip :content="$t('card.detail.workloadCountTip')">
                  <help-circle-icon class="resource-card-help-icon" />
                </t-tooltip>
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
          <VChart
            :option="
              {
                ...getRangeOptions([
                  {
                    name: t('dashboard.allocRateLegend'),
                    data: gaugeConfig[0]?.data,
                    itemStyle: {
                      color: '#5B8FF9',
                      borderColor: '#5B8FF9',
                    },
                    lineStyle: {
                      width: 3,
                      color: '#5B8FF9',
                    },
                  },
                  {
                    name: t('dashboard.usageRateLegend'),
                    data: gaugeConfig[2]?.data,
                    itemStyle: {
                      color: '#42C090',
                      borderColor: '#42C090',
                    },
                    lineStyle: {
                      width: 3,
                      color: '#42C090',
                    },
                  },
                ]),
                animation: false,
              }
            "
            :autoresize="true"
          />
        </div>
      </block-box>
      <block-box :title="$t('dashboard.gpuMemAllocUsageTrend')">
        <div class="trend-chart">
          <VChart
            :option="
              {
                ...getRangeOptions([
                  {
                    name: t('dashboard.allocRateLegend'),
                    data: gaugeConfig[1]?.data,
                    itemStyle: {
                      color: '#5B8FF9',
                      borderColor: '#5B8FF9',
                    },
                    lineStyle: {
                      width: 3,
                      color: '#5B8FF9',
                    },
                  },
                  {
                    name: t('dashboard.usageRateLegend'),
                    data: gaugeConfig[3]?.data,
                    itemStyle: {
                      color: '#42C090',
                      borderColor: '#42C090',
                    },
                    lineStyle: {
                      width: 3,
                      color: '#42C090',
                    },
                  },
                ]),
                animation: false,
              }
            "
            :autoresize="true"
          />
        </div>
      </block-box>

      <block-box :title="title" v-for="{ title, data, unit, seriesNameKey } in lineToolsView" :key="title">
        <div class="trend-chart">
          <VChart :option="getLineOptions2({ data, unit, seriesName: t(seriesNameKey), animation: false })" :autoresize="true" />
        </div>
      </block-box>
    </div>

  </div>
</template>

<script setup lang="jsx">
import PageHeader from '@/components/PageHeader.vue';
import { useRoute, useRouter } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { onMounted, ref, watch, computed } from 'vue';
import { HelpCircleIcon } from 'tdesign-icons-vue-next';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import VChart from 'vue-echarts';
import cardApi from '~/vgpu/api/card';
import nodeApi from '~/vgpu/api/node';
import WorkloadSemiProgress from './components/WorkloadSemiProgress.vue';
import { timeParse, calculatePrometheusStep, roundToDecimal, getResourceColor } from '@/utils';
import { getLineOptions as getLineOptions2 } from '~/vgpu/components/config';
import { getRangeOptions } from '../../monitor/overview/getOptions';
import { useI18n } from 'vue-i18n';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const detail = ref({});
const nodeUid = ref('');
const CARD_TYPE_ICON_MAP = {
  NVIDIA: 'gpu-nvidia',
  MXC: 'gpu-nvidia',
  ASCEND: 'gpu-ascend',
  METAX: 'gpu-metax',
  AWS: 'gpu-aws',
  AWSNEURON: 'gpu-aws',
};

const extractLetters = (value) => {
  if (typeof value === 'string') {
    return value.match(/[a-z]+/gi) || [];
  }
  return [];
};

const gpuTypeIcon = computed(() => {
  const vendor = extractLetters(detail.value?.type)?.[0]?.toUpperCase();
  if (!vendor) return '';
  return CARD_TYPE_ICON_MAP[vendor] || 'GPU';
});
const getCardStatusDisplay = ({ health, isExternal }) => {
  if (isExternal || health === undefined || health === null) {
    return { icon: 'status-unmanaged', text: t('card.unknown') };
  }
  if (health) {
    return { icon: 'status-schedulable', text: t('card.normal') };
  }
  return { icon: 'status-unschedulable', text: t('card.abnormal') };
};
const headerStatusDisplay = computed(() => getCardStatusDisplay(detail.value || {}));

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const basicPowerText = computed(() => {
  const v = lineTools.value[1]?.percent;
  const unit = lineTools.value[1]?.gaugeUnit || 'W';
  if (v === undefined || v === null) return '--';
  return `${Number(v)} ${unit}`;
});
const basicTemperatureText = computed(() => {
  const v = lineTools.value[0]?.percent;
  const unit = lineTools.value[0]?.gaugeUnit || '℃';
  if (v === undefined || v === null) return '--';
  return `${Number(v)} ${unit}`;
});

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
  return Number.isFinite(total) ? `${roundToDecimal(total / 100, 1)}` : '--';
});

const memoryTotalText = computed(() => {
  const total = Number(gaugeConfig.value?.[1]?.total);
  return Number.isFinite(total) ? `${roundToDecimal(total, 1)} GiB` : '--';
});

const formatUsedValue = (v, unit, divisor = 1) => {
  const n = Number(v);
  if (!Number.isFinite(n)) return '--';
  const normalizedDivisor = Number(divisor) > 0 ? Number(divisor) : 1;
  const text = `${roundToDecimal(n / normalizedDivisor, 1)}`;
  return unit && String(unit).trim() ? `${text} ${unit}` : text;
};

const computeAllocUsedText = computed(() => (detail.value?.isExternal ? '--' : formatUsedValue(gaugeConfig.value?.[0]?.used, gaugeConfig.value?.[0]?.unit, 100)));
const computeUsageUsedText = computed(() => formatUsedValue(gaugeConfig.value?.[2]?.used, gaugeConfig.value?.[2]?.unit, 100));
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
const workloadCountUsed = computed(() => Number(detail.value?.vgpuUsed));
const workloadCountTotal = computed(() => Number(detail.value?.vgpuTotal));
const workloadCountUsedText = computed(() => (Number.isFinite(workloadCountUsed.value) ? `${workloadCountUsed.value}` : '--'));
const workloadCountTotalText = computed(() => (Number.isFinite(workloadCountTotal.value) ? `${workloadCountTotal.value}` : '--'));
const workloadCountPercentRaw = computed(() => {
  if (!Number.isFinite(workloadCountUsed.value) || !Number.isFinite(workloadCountTotal.value) || workloadCountTotal.value <= 0) {
    return 0;
  }
  return (workloadCountUsed.value / workloadCountTotal.value) * 100;
});
const workloadCountPercentProgress = computed(() => clampPercent(workloadCountPercentRaw.value));

const lineTools = ref([
  {
    titleKey: 'card.detail.gpuTemperatureTrend',
    seriesNameKey: 'card.detail.gpuTemperature',
    query: `avg by (device_no,driver_version) (hami_device_temperature{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: '℃',
    gaugeUnit: '℃',
    percent: undefined,
    total: 0,
    hideInfo: true,
    showProgress: false,
  },
  {
    titleKey: 'card.detail.gpuPowerTrend',
    seriesNameKey: 'card.detail.gpuPower',
    query: `avg by (device_no,driver_version) (hami_device_power{deviceuuid=~"$deviceuuid"})`,
    data: [],
    unit: 'W',
    gaugeUnit: 'W',
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

const handleToNodeDetail = () => {
  if (!nodeUid.value) return;
  router.push(`/admin/vgpu/node/admin/${nodeUid.value}?nodeName=${detail.value?.nodeName || ''}`);
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

  .basic-info-row {
    display: flex;
    gap: 8px;
    margin-top: 12px;
  }

  .basic-info-card {
    display: flex;
    flex: 1;
    flex-direction: column;
    justify-content: center;
    min-width: 0;
    gap: 2px;
    padding: 15px 20px;
    border-radius: 8px;
    background: #f5f7fa;
    border: 0;
    overflow-x: hidden;
  }

  .basic-info-title {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    font-size: 16px;
    font-weight: 500;
    color: #324558;
    line-height: 28px;
    cursor: default;

    .text {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .basic-info-share {
    display: inline-flex;
    align-items: center;
  }

  .basic-info-subtitle {
    color: #939ea9;
    font-size: 12px;
    line-height: 20px;
  }

.gpu-type-icon {
  width: 16px;
  height: 16px;
  color: #64748b;
}
}

.resource-overview-cards {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  gap: 8px;
}

.resource-overview-card {
  flex: 1;
  display: flex;
}

.progress-wrapper {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
}

.resource-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  min-height: 0;
  width: 100%;
  padding: 15px 20px;
  background: #f5f7fa;
  border-radius: 8px;
  border: 0;
  overflow-x: hidden;
}

.resource-card-header {
  display: flex;
  align-items: center;
  gap: 20px;
  width: 100%;
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

.workload-progress-summary {
  margin-top: -76px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: #939ea9;
}

.workload-progress-value {
  line-height: 28px;

  b {
    font-size: 20px;
    color: #324558;
  }
}

.workload-progress-subtitle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.resource-card-help-icon {
  color: #939ea9;
  font-size: 14px;
  cursor: pointer;
}

.resource-card-footer {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  margin-top: 15px;
  width: 100%;
}

.resource-card-rate-wrap {
  width: 100%;
  background: #ffffff;
  border-radius: 6px;
  padding: 10px 12px;
  box-sizing: border-box;
  box-shadow: 0 4px 10px rgba(2, 5, 8, 0.06);
}

.resource-card-footer-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.resource-card-footer-title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
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
  height: 100%;
  margin-top: 0;
}

.line-box {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;

  > .home-block {
    flex: 1 1 calc(50% - 10px);
    min-width: 0;
    height: 320px;
    padding: 16px 20px;
    margin-bottom: 0;
    display: flex;
    flex-direction: column;
  }

  > .home-block :deep(.home-block-content) {
    flex: 1;
    min-height: 0;
  }
}

.node-block {
  display: flex;
  flex-direction: column;
  box-shadow: none;
  .home-block-content {
    flex: 1;
  }
}

.resource-overview-block {
  box-shadow: none;
}
</style>
