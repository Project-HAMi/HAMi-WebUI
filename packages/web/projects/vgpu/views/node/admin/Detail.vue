<template>
  <div>
    <page-header
      :title="$t('node.detail.title')"
      :name="detail.name"
      :status="headerStatusDisplay.text"
      :status-icon="headerStatusDisplay.icon"
    />

    <section class="node-overview-wrap">
      <div class="node-overview-panel">
        <div class="node-detail" :class="{ 'is-en': locale.startsWith('en') }">
          <div class="node-detail-left">
            <div class="title">{{ $t('node.detail.nodeOverview') }}</div>
            <div class="node-detail-info-summary node-detail-info-summary-cols">
              <div
                v-for="(group, groupIndex) in detailColumnGroups"
                :key="groupIndex"
                class="summary-col"
              >
                <div
                  v-for="{ label, value, render } in group"
                  :key="label"
                  class="summary-item"
                >
                  <div class="summary-item-label">{{ label }}</div>
                  <div v-if="render" class="summary-item-value">
                    <component :is="render(detail)" />
                  </div>
                  <div v-else class="summary-item-value">{{ detail[value] }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="node-overview-panel node-workload-panel">
        <div class="title">{{ $t('node.detail.resourceOverview') }}</div>
        <ul class="node-system-resource-cards">
          <li class="resource-overview-card">
            <div class="resource-card">
              <div class="resource-card-header">
                <div class="resource-card-icon">
                  <svg-icon icon="vgpu-core" />
                </div>
                <div class="resource-card-header-info">
                  <div class="resource-card-value resource-card-value--compute">
                    {{ computePowerTotalText }}
                  </div>
                  <div class="resource-card-sub-title">
                    {{ $t('dashboard.computePowerTotal') }}
                  </div>
                </div>
              </div>

              <div class="resource-card-footer">
                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">{{ $t('dashboard.allocRateLegend') }}</div>
                    <div class="resource-card-footer-value">
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
                    <div class="resource-card-footer-title">{{ $t('dashboard.usageRateLegend') }}</div>
                    <div class="resource-card-footer-value">
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
                  <svg-icon icon="vgpu-mem" />
                </div>
                <div class="resource-card-header-info">
                  <div class="resource-card-value resource-card-value--compute">
                    {{ gpuMemoryTotalText }}
                  </div>
                  <div class="resource-card-sub-title">
                    {{ $t('dashboard.memoryTotal') }}
                  </div>
                </div>
              </div>

              <div class="resource-card-footer">
                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">{{ $t('dashboard.allocRateLegend') }}</div>
                    <div class="resource-card-footer-value">
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
                    <div class="resource-card-footer-title">{{ $t('dashboard.usageRateLegend') }}</div>
                    <div class="resource-card-footer-value">
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
      </div>
    </section>

    <trend-time-filter v-model="times" />

    <div class="line-box">
      <block-box :title="$t('dashboard.gpuComputeAllocUsageTrend')">
        <div class="trend-chart">
          <VChart
              :option="
              getRangeOptions({
                allocation: gaugeConfig[0].data,
                usage: gaugeConfig[2].data,
              }, t)
            "
            :autoresize="true"
          />
        </div>
      </block-box>
      <block-box :title="$t('dashboard.gpuMemAllocUsageTrend')">
        <div class="trend-chart">
          <VChart
            :option="
              getRangeOptions({
                allocation: gaugeConfig[1].data,
                usage: gaugeConfig[3].data,
              }, t)
            "
            :autoresize="true"
          />
        </div>
      </block-box>
    </div>
  </div>
</template>

<script setup lang="jsx">
import PageHeader from '@/components/PageHeader.vue';
import { useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { computed, onMounted, ref } from 'vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import VChart from 'vue-echarts';
import nodeApi from '~/vgpu/api/node';
import { getRangeOptions } from './getOptions';
import { useI18n } from 'vue-i18n';
import { getResourceColor, roundToDecimal } from '@/utils';

const route = useRoute();
const { t, locale } = useI18n();

const detail = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const _gaugeConfigBase = [
  {
    titleKey: 'dashboard.computeAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vcore_allocated{node=~"$node"}) by (instance))`,
    totalQuery: `avg(sum(hami_core_size{node=~"$node"}) by (instance))`,
    percentQuery: `avg(sum(hami_container_vcore_allocated{node=~"$node"}) by (instance)) / avg(sum(hami_core_size{node=~"$node"}) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vmemory_allocated{node=~"$node"}) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size{node=~"$node"}) by (instance)) / 1024`,
    percentQuery: `(avg(sum(hami_container_vmemory_allocated{node=~"$node"}) by (instance)) / 1024) /(avg(sum(hami_memory_size{node=~"$node"}) by (instance)) / 1024) *100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
  {
    titleKey: 'dashboard.computeUsageRate',
    percent: 0,
    query: `avg(sum(hami_core_util{node=~"$node"}) by (instance))`,
    percentQuery: `avg(sum(hami_core_util_avg{node=~"$node"}) by (instance))`,
    totalQuery: `avg(sum(hami_core_size{node=~"$node"}) by (instance))`,
    total: 100,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memUsageRate',
    percent: 0,
    query: `avg(sum(hami_memory_used{node=~"$node"}) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size{node=~"$node"}) by (instance))/1024`,
    percentQuery: `(avg(sum(hami_memory_used{node=~"$node"}) by (instance)) / 1024)/(avg(sum(hami_memory_size{node=~"$node"}) by (instance))/1024)*100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
];

const gaugeData = useInstantVector(
  _gaugeConfigBase.map(item => ({ ...item, title: t(item.titleKey) })),
  (query) => query.replaceAll(`$node`, detail.value.name),
  times,
);

const gaugeConfig = computed(() =>
  gaugeData.value.map((item) => ({
    ...item,
    title: item.titleKey ? t(item.titleKey) : item.title,
  })),
);

const computePowerTotal = computed(() => {
  const total = Number(gaugeConfig.value?.[0]?.total);
  return Number.isFinite(total) ? roundToDecimal(total, 1) : undefined;
});

const gpuMemoryTotal = computed(() => {
  const total = Number(gaugeConfig.value?.[1]?.total);
  return Number.isFinite(total) ? roundToDecimal(total, 1) : undefined;
});

const computeAllocPercentRaw = computed(() => {
  if (detail.value?.isExternal) return undefined;
  const percent = Number(gaugeConfig.value?.[0]?.percent);
  return Number.isFinite(percent) ? percent : undefined;
});

const computeUsagePercentRaw = computed(() => {
  const percent = Number(gaugeConfig.value?.[2]?.percent);
  return Number.isFinite(percent) ? percent : undefined;
});

const memoryAllocPercentRaw = computed(() => {
  if (detail.value?.isExternal) return undefined;
  const percent = Number(gaugeConfig.value?.[1]?.percent);
  return Number.isFinite(percent) ? percent : undefined;
});

const memoryUsagePercentRaw = computed(() => {
  const percent = Number(gaugeConfig.value?.[3]?.percent);
  return Number.isFinite(percent) ? percent : undefined;
});

const clampPercent = (v) => Math.max(0, Math.min(100, v));
const roundPercentForProgress = (p) => (p === undefined ? undefined : roundToDecimal(p, 2));
const buildPercentViews = (rawComputed) => {
  const progress = computed(() => {
    const p = rawComputed.value;
    return p === undefined ? undefined : clampPercent(p);
  });
  const progressRounded = computed(() => roundPercentForProgress(progress.value));
  const text = computed(() => {
    const p = rawComputed.value;
    return p === undefined ? '--' : `${Number(p).toFixed(2)}%`;
  });
  return { progress, progressRounded, text };
};

const {
  progress: computeAllocPercentProgress,
  progressRounded: computeAllocPercentProgressRounded,
  text: computeAllocPercentText,
} = buildPercentViews(computeAllocPercentRaw);
const {
  progress: computeUsagePercentProgress,
  progressRounded: computeUsagePercentProgressRounded,
  text: computeUsagePercentText,
} = buildPercentViews(computeUsagePercentRaw);
const {
  progress: memoryAllocPercentProgress,
  progressRounded: memoryAllocPercentProgressRounded,
  text: memoryAllocPercentText,
} = buildPercentViews(memoryAllocPercentRaw);
const {
  progress: memoryUsagePercentProgress,
  progressRounded: memoryUsagePercentProgressRounded,
  text: memoryUsagePercentText,
} = buildPercentViews(memoryUsagePercentRaw);

const computePowerTotalText = computed(() => {
  const v = computePowerTotal.value;
  return v === undefined ? '--' : `${roundToDecimal(v / 100, 1)}`;
});

const gpuMemoryTotalText = computed(() => {
  const v = gpuMemoryTotal.value;
  return v === undefined ? '--' : `${Number(v).toFixed(2)} GiB`;
});

const headerStatusDisplay = computed(() =>
  getNodeStatusDisplay({
    isSchedulable: detail.value?.isSchedulable,
    isExternal: detail.value?.isExternal,
  }),
);

const getNodeStatusDisplay = ({ isSchedulable, isExternal }) => {
  if (isExternal || isSchedulable === undefined || isSchedulable === null) {
    return { icon: 'status-unmanaged', text: t('node.unknown') };
  }
  if (isSchedulable) {
    return { icon: 'status-schedulable', text: t('node.normal') };
  }
  return { icon: 'status-unschedulable', text: t('node.abnormal') };
};


const toDisplayText = (value) => (value === '' || value === undefined || value === null ? '--' : value);

const detailColumns = computed(() => [
  {
    label: t('node.detail.nodeIpAddress'),
    value: 'ip',
    render: ({ ip }) => <span>{ip || '--'}</span>,
  },
  {
    label: t('node.detail.nodeUuid'),
    value: 'uid',
    render: ({ uid }) => (
      <ellipsis-text text={uid || '--'} mode="middle" tooltip="always" />
    ),
  },
  {
    label: t('node.detail.osVersion'),
    value: 'osImage',
    render: ({ osImage }) => (
      <span>{toDisplayText(osImage)}</span>
    ),
  },
  {
    label: t('node.detail.kubeletVersion'),
    value: 'kubeletVersion',
    render: ({ kubeletVersion }) => (
      <span>{toDisplayText(kubeletVersion)}</span>
    ),
  },
  {
    label: t('node.detail.containerRuntime'),
    value: 'containerRuntimeVersion',
    render: ({ containerRuntimeVersion }) => (
      <span>{toDisplayText(containerRuntimeVersion)}</span>
    ),
  },
  {
    label: t('node.detail.architecture'),
    value: 'architecture',
    render: ({ architecture }) => (
      <span>{toDisplayText(architecture)}</span>
    ),
  },
  {
    label: t('node.detail.kernelVersion'),
    value: 'kernelVersion',
    render: ({ kernelVersion }) => (
      <span>{toDisplayText(kernelVersion)}</span>
    ),
  },
  {
    label: t('node.detail.creationTime'),
    value: 'creationTimestamp',
    render: ({ creationTimestamp }) => (
      <span>{toDisplayText(creationTimestamp)}</span>
    ),
  },
]);

const detailColumnGroups = computed(() => {
  const columns = detailColumns.value || [];
  const mid = Math.ceil(columns.length / 2);
  return [columns.slice(0, mid), columns.slice(mid)];
});

const refresh = async () => {
  detail.value = await nodeApi.getNodeDetail({ uid: route.params.uid });
};

onMounted(async () => {
  await refresh();
});
</script>

<style scoped lang="scss">
.node-overview-wrap {
  display: flex;
  margin-top: 16px;
  margin-bottom: 15px;
  gap: 16px;
}

.node-overview-panel {
  flex: 1 1 48%;
  min-width: 0;
  padding: 12px 16px 16px;
  border: 1px solid #e4ebf1;
  border-radius: 12px;
  background: #fff;
}

.title {
  color: #1d2b3a;
  font-size: 16px;
  font-weight: 500;
  line-height: 28px;
  margin-bottom: 8px;
}

.node-detail {
  display: flex;
  width: 100%;
  height: 100%;

  .node-detail-left {
    width: 100%;
  }

  .node-detail-info-summary {
    width: 100%;
    margin-top: 8px;

    &-cols {
      display: flex;
      flex-flow: row nowrap;
      gap: 12px;
    }
  }

  .summary-col {
    flex: 1;
    min-width: 0;
    padding: 8px 12px;
    border-radius: 8px;
    background: rgb(0 0 0 / 2%);
  }

  .summary-item {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    margin-bottom: 6px;

    &:last-child {
      margin-bottom: 0;
    }
  }

  .summary-item-label {
    color: #939ea9;
    font-size: 12px;
    font-weight: 400;
    line-height: 16px;

    &::after {
      content: ': ';
    }
  }

  .summary-item-value {
    color: #1d2b3a;
    font-size: 14px;
    font-weight: 500;
    line-height: 20px;
    min-width: 0;

    :deep(.ellipsis-text) {
      margin-right: 8px;
    }
  }

  &.is-en {
    .summary-item-label {
      word-break: break-word;
    }
  }
}

.node-workload-panel {
  min-width: 0;
}

.node-system-resource-cards {
  margin: 10px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: row;
  gap: 12px;

  .resource-overview-card {
    flex: 1;
    min-width: 0;
  }
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
  font-size: 12px;
  color: #939ea9;
  line-height: 20px;
}

.resource-card-footer-value {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.resource-card-footer-percent {
  font-size: 14px;
  font-weight: 500;
  color: #1d2b3a;
}

.trend-chart {
  height: 100%;
  margin-top: 0;
  min-width: 0;
}

.line-box {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: 16px;
  row-gap: 16px;

  > .home-block {
    min-width: 0;
    height: 320px;
    display: flex;
    flex-direction: column;
  }

  > .home-block :deep(.home-block-content) {
    flex: 1;
    min-height: 0;
  }

  > .home-block :deep(.home-block-content) > .trend-chart {
    height: 100%;
  }
}

.node-block {
  display: flex;
  flex-direction: column;
  box-shadow: none;
  margin-bottom: 15px;
  .home-block-content {
    flex: 1;
  }
}

@media (max-width: 1200px) {
  .node-overview-wrap {
    flex-direction: column;
  }

  .node-detail .node-detail-info-summary-cols {
    flex-direction: column;
  }

  .node-system-resource-cards {
    flex-direction: column;
  }
}
</style>
