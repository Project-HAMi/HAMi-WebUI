<template>
  <div>
    <back-header>
      {{ $t('node.detail.title') }} : {{ detail.name }}
    </back-header>

    <block-box class="node-block">
      <div class="node-detail" :class="{ 'is-en': locale.startsWith('en') }">
        <div class="node-detail-left">
          <div class="title">{{ $t('node.detail.detailInfo') }}</div>
          <ul class="node-detail-info">
            <li v-for="{ label, value, render } in detailColumns" :key="label">
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

    <block-box :title="$t('node.detail.resourceOverview')">
      <ul class="resource-overview-cards">
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
                <div class="resource-card-value resource-card-value--compute">{{ gpuMemoryTotalText }}</div>
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
    </block-box>

    <trend-time-filter v-model="times" />

    <div class="line-box">
      <block-box :title="$t('dashboard.gpuComputeAllocUsageTrend')">
        <div class="trend-chart">
          <echarts-plus
              :options="
              getRangeOptions({
                allocation: gaugeConfig[0].data,
                usage: gaugeConfig[2].data,
              }, t)
            "
          />
        </div>
      </block-box>
      <block-box :title="$t('dashboard.gpuMemAllocUsageTrend')">
        <div class="trend-chart">
          <echarts-plus
            :options="
              getRangeOptions({
                allocation: gaugeConfig[1].data,
                usage: gaugeConfig[3].data,
              }, t)
            "
          />
        </div>
      </block-box>
    </div>

    <block-box :title="$t('routes.cards')">
      <CardList
        v-if="detail.uid && detail.name"
        :key="`${locale}-${detail.uid}`"
        :hideTitle="true"
        :filters="{ nodeName: detail.name }"
      />
    </block-box>

    <block-box :title="$t('routes.tasks')">
      <template v-if="detail.isExternal">
        <el-alert :title="$t('node.detail.unmanagedNoTask')" show-icon type="warning" :closable="false" />
        <el-empty :description="$t('node.detail.noTaskData')" :image-size="100" />
      </template>
      <template v-else>
        <TaskList
          v-if="detail.uid"
          :key="`${locale}-${detail.uid}`"
          :hideTitle="true"
          :filters="{ nodeUid: detail.uid, nodeName: detail.name }"
        />
      </template>
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import { useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { computed, onMounted, ref } from 'vue';
import CardList from '~/vgpu/views/card/admin/index.vue';
import TaskList from '~/vgpu/views/task/admin/index.vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import EchartsPlus from '@/components/Echarts-plus.vue';
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

const computeAllocPercentProgress = computed(() => {
  const p = computeAllocPercentRaw.value;
  return p === undefined ? undefined : clampPercent(p);
});

const computeUsagePercentProgress = computed(() => {
  const p = computeUsagePercentRaw.value;
  return p === undefined ? undefined : clampPercent(p);
});

const memoryAllocPercentProgress = computed(() => {
  const p = memoryAllocPercentRaw.value;
  return p === undefined ? undefined : clampPercent(p);
});

const memoryUsagePercentProgress = computed(() => {
  const p = memoryUsagePercentRaw.value;
  return p === undefined ? undefined : clampPercent(p);
});

const computeAllocPercentProgressRounded = computed(() => roundPercentForProgress(computeAllocPercentProgress.value));
const computeUsagePercentProgressRounded = computed(() => roundPercentForProgress(computeUsagePercentProgress.value));
const memoryAllocPercentProgressRounded = computed(() => roundPercentForProgress(memoryAllocPercentProgress.value));
const memoryUsagePercentProgressRounded = computed(() => roundPercentForProgress(memoryUsagePercentProgress.value));

const computePowerTotalText = computed(() => {
  const v = computePowerTotal.value;
  return v === undefined ? '--' : `${v}`;
});

const gpuMemoryTotalText = computed(() => {
  const v = gpuMemoryTotal.value;
  return v === undefined ? '--' : `${v} GiB`;
});

const computeAllocPercentText = computed(() => {
  const p = computeAllocPercentRaw.value;
  return p === undefined ? '--' : `${roundToDecimal(p, 2)}%`;
});

const computeUsagePercentText = computed(() => {
  const p = computeUsagePercentRaw.value;
  return p === undefined ? '--' : `${roundToDecimal(p, 2)}%`;
});

const memoryAllocPercentText = computed(() => {
  const p = memoryAllocPercentRaw.value;
  return p === undefined ? '--' : `${roundToDecimal(p, 2)}%`;
});

const memoryUsagePercentText = computed(() => {
  const p = memoryUsagePercentRaw.value;
  return p === undefined ? '--' : `${roundToDecimal(p, 2)}%`;
});

const detailColumns = computed(() => [
  {
    label: t('node.status'),
    value: 'status',
    render: ({ isSchedulable, isExternal }) => {
      if (!detail.value) {
        return <el-tag disable-transitions size="small" type="info">{t('node.detail.loading')}</el-tag>;
      }
      const icon = isExternal || isSchedulable === undefined || isSchedulable === null
        ? 'status-unmanaged'
        : (isSchedulable ? 'status-schedulable' : 'status-unschedulable');
      const text = isExternal || isSchedulable === undefined || isSchedulable === null
        ? t('node.unknown')
        : (isSchedulable ? t('node.normal') : t('node.abnormal'));
      return (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <svg-icon icon={icon} style={{ fontSize: '16px' }} />
          <span>{text}</span>
        </span>
      );
    },
  },
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
    label: t('node.detail.osType'),
    value: 'operatingSystem',
    render: ({ operatingSystem }) => (
        <span>
          {operatingSystem==='' ? '--' : operatingSystem}
        </span>
    ),
  },
  {
    label: t('node.detail.architecture'),
    value: 'architecture',
    render: ({ architecture }) => (
        <span>
          {architecture==='' ? '--' : architecture}
        </span>
    ),
  },
  {
    label: t('node.detail.kubeletVersion'),
    value: 'kubeletVersion',
    render: ({ kubeletVersion }) => (
        <span>
          {kubeletVersion==='' ? '--' : kubeletVersion}
        </span>
    ),
  },
  {
    label: t('node.detail.osVersion'),
    value: 'osImage',
    render: ({ osImage }) => (
        <span>
          {osImage==='' ? '--' : osImage}
        </span>
    ),
  },
  {
    label: t('node.detail.kernelVersion'),
    value: 'kernelVersion',
    render: ({ kernelVersion }) => (
        <span>
          {kernelVersion==='' ? '--' : kernelVersion}
        </span>
    ),
  },
  {
    label: t('node.detail.kubeProxyVersion'),
    value: 'kubeProxyVersion',
    render: ({ kubeProxyVersion }) => (
        <span>
          {kubeProxyVersion==='' ? '--' : kubeProxyVersion}
        </span>
    ),
  },
  {
    label: t('node.detail.containerRuntime'),
    value: 'containerRuntimeVersion',
    render: ({ containerRuntimeVersion }) => (
        <span>
          {containerRuntimeVersion==='' ? '--' : containerRuntimeVersion}
        </span>
    ),
  },
  {
    label: t('node.cardCount'),
    value: 'cardCnt',
    render: ({ cardCnt }) => (
        <span>
          {cardCnt==='' ? '--' : cardCnt}
        </span>
    ),
  },
  {
    label: t('node.detail.creationTime'),
    value: 'creationTimestamp',
    render: ({ creationTimestamp }) => (
        <span>
          {creationTimestamp==='' ? '--' : creationTimestamp}
        </span>
    ),
  },
]);

const refresh = async () => {
  detail.value = await nodeApi.getNodeDetail({ uid: route.params.uid });
};

onMounted(async () => {
  await refresh();
});
</script>

<style scoped lang="scss">
.node-detail {
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
    margin-bottom: 15px;
  }
  .node-detail-left {
    width: 100%;
  }

  .node-detail-info {
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
      width: 110px;
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
    .node-detail-info {
      .label {
        width: 145px;
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
  gap: 10px;
}

.resource-card-footer-percent {
  font-size: 14px;
  font-weight: 500;
  color: #1d2b3a;
}

.trend-chart {
  height: 200px;
  margin-top: 15px;
  min-width: 0;
}

.line-box {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: 16px;

  > .home-block {
    min-width: 0;
  }
}

.node-block {
  display: flex;
  flex-direction: column;
  margin-bottom: 15px;
  .home-block-content {
    flex: 1;
  }
}
</style>
