<template>
  <page-header
    :title="$t('task.workload')"
    :name="detail.appName || detail.name"
    :status="headerStatusDisplay.text"
    :status-icon="headerStatusDisplay.icon"
  >
    <template #titleSuffix>
      <ElPopover
        v-if="detail.status === 'unknown' || detail.status === 'failed'"
        placement="top"
        trigger="hover"
        popper-style="width: 180px"
      >
        <template #reference>
          <el-icon color="#939EA9" size="14">
            <QuestionFilled />
          </el-icon>
        </template>
        <span style="margin-left: 5px">{{ $t('task.checkCloudPlatform') }}</span>
      </ElPopover>
    </template>
  </page-header>

  <div class="task-top">
    <block-box :title="$t('task.detail.detailInfo')" class="basic-info-block">
      <div class="task-detail" :class="{ 'is-en': locale.startsWith('en') }">
        <div class="left">
          <div class="basic-info-cards">
            <div class="basic-info-card">
              <div class="basic-info-card-title">{{ detail.namespace || '--' }}</div>
              <div class="basic-info-card-sub-title">{{ $t('task.namespace') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-card-title node-link" @click="handleNodeJump">
                <ellipsis-text :text="detail.nodeName || '--'" mode="middle" tooltip="always" />
                <svg-icon icon="jump" />
              </div>
              <div class="basic-info-card-sub-title">{{ $t('task.node') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-card-title">
                <div v-if="gpuModelList.length === 0">--</div>
                <div v-else class="gpu-model-container">
                  <template v-for="(item, index) in gpuModelList" :key="`${item.model}-${index}`">
                    <div class="gpu-model-item">
                      <t-tag theme="default" size="medium" variant="light">{{ item.model }}</t-tag>
                      <span class="gpu-count">× {{ item.count }}</span>
                    </div>
                    <span v-if="index < gpuModelList.length - 1" class="gpu-separator"></span>
                  </template>
                </div>
              </div>
              <div class="basic-info-card-sub-title">{{ $t('task.gpuModel') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-card-title">
                <span class="related-gpu-count-text">{{ relatedGpuCountText }}</span>
                <t-popup
                  v-if="safeDeviceIds.length > 0"
                  destroy-on-close
                  theme="light"
                  trigger="click"
                  placement="bottom"
                  show-arrow
                  overlay-class-name="relative-gpu-tooltip"
                >
                  <template #default>
                    <svg-icon icon="related-gpu-eye" class="gpu-eye-icon" />
                  </template>
                  <template #content>
                    <div class="relative-gpu-tooltip-content">
                      <div class="popup-title">{{ $t('task.relatedGpu') }}：</div>
                      <t-table row-key="uuid" :data="relatedGpuTableData" :columns="relatedGpuTableColumns" />
                    </div>
                  </template>
                </t-popup>
              </div>
              <div class="basic-info-card-sub-title">{{ $t('task.relatedGpu') }}</div>
            </div>
          </div>
          <div class="basic-info-summary">
            <div class="summary-item">
              <span class="summary-item-label">{{ $t('task.image') }}</span>
              <span class="summary-item-value">
                <TTooltip v-if="basicImageTooltip" :content="basicImageTooltip">
                  <span>{{ basicImage }}</span>
                </TTooltip>
                <span v-else>{{ basicImage }}</span>
              </span>
            </div>
            <div class="summary-item">
              <span class="summary-item-label">{{ $t('task.createTime') }}</span>
              <span class="summary-item-value">{{ basicCreateTime }}</span>
            </div>
          </div>
        </div>
      </div>
    </block-box>
  </div>

  <block-box :title="$t('task.detail.resourceOverview')" class="workload-overview">
    <div class="row">
      <div class="row-card">
        <div class="row-card-content">
          <div class="row-card-content-icon"><svg-icon icon="node-memory-total" /></div>
          <div class="row-card-content-info">
            <div class="row-card-title">{{ resourceOverviewTexts.gpuCards }}</div>
            <div class="row-card-sub-title">{{ $t('task.gpuCardCount') }}</div>
          </div>
        </div>
      </div>
      <div class="row-card">
        <div class="row-card-content">
          <div class="row-card-content-icon"><svg-icon icon="node-cpu-total" /></div>
          <div class="row-card-content-info">
            <div class="row-card-title">{{ resourceOverviewTexts.computeLimit }}</div>
            <div class="row-card-sub-title">{{ $t('task.computePowerLimit') }}</div>
          </div>
        </div>
      </div>
      <div class="row-card">
        <div class="row-card-content">
          <div class="row-card-content-icon"><svg-icon icon="vgpu-mem" /></div>
          <div class="row-card-content-info">
            <div class="row-card-title">{{ resourceOverviewTexts.singleCardMemory }}</div>
            <div class="row-card-sub-title">{{ $t('task.singleCardMemory') }}</div>
          </div>
        </div>
      </div>
      <div class="row-card">
        <div class="row-card-content">
          <div class="row-card-content-icon"><svg-icon icon="cpu-limit" /></div>
          <div class="row-card-content-info">
            <div class="row-card-title">{{ resourceOverviewTexts.cpuLimit }}</div>
            <div class="row-card-sub-title">{{ $t('task.cpuLimit') }}</div>
          </div>
        </div>
      </div>
      <div class="row-card">
        <div class="row-card-content">
          <div class="row-card-content-icon"><svg-icon icon="card-id" /></div>
          <div class="row-card-content-info">
            <div class="row-card-title">{{ resourceOverviewTexts.memoryLimit }}</div>
            <div class="row-card-sub-title">{{ $t('task.memoryLimit') }}</div>
          </div>
        </div>
      </div>
    </div>
  </block-box>

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
          <VChart
            :option="getLineOptions({ data, seriesName: $t('dashboard.usageRateLegend'), animation: false })"
            :autoresize="true"
            class="trend-vchart"
          />
        </template>
      </div>
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import PageHeader from '@/components/PageHeader.vue';
import { useRoute, useRouter } from 'vue-router';
import { onMounted, ref, watch, computed } from 'vue';
import { Tooltip as TTooltip } from 'tdesign-vue-next';
import useInstantVector from '~/vgpu/hooks/useInstantVector';

import cardApi from '~/vgpu/api/card';
import nodeApi from '~/vgpu/api/node';
import { QuestionFilled } from '@element-plus/icons-vue';
import { ElPopover } from 'element-plus';
import { timeParse, calculatePrometheusStep, roundToDecimal } from '@/utils';
import taskApi from '~/vgpu/api/task';
import BlockBox from '@/components/BlockBox.vue';
import { getLineOptions } from '~/vgpu/components/config';
import VChart from 'vue-echarts';
import { useI18n } from 'vue-i18n';

const route = useRoute();
const router = useRouter();
const { t, locale } = useI18n();

const detail = ref({});
const nodeUid = ref('');
const cardTypeById = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const getStatusDisplay = (status) => {
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
  return enums[status] || enums.unknown;
};

const safeDeviceIds = computed(() => (
  Array.isArray(detail.value?.deviceIds) ? detail.value.deviceIds : []
));
const gpuModelList = computed(() => {
  if (!safeDeviceIds.value.length) return [];
  const grouped = new Map();
  safeDeviceIds.value.forEach((id) => {
    const model = cardTypeById.value?.[id] || detail.value?.type || '--';
    grouped.set(model, (grouped.get(model) || 0) + 1);
  });
  return Array.from(grouped.entries()).map(([model, count]) => ({ model, count }));
});
const relatedGpuCountText = computed(() => t('task.relatedGpuCards', { count: safeDeviceIds.value.length }));
const relatedGpuTableData = computed(() => safeDeviceIds.value.map((uuid) => ({
  model: cardTypeById.value?.[uuid] || detail.value?.type || '--',
  uuid,
})));
const relatedGpuTableColumns = computed(() => [
  {
    colKey: 'model',
    title: t('task.gpuModel'),
    width: 104,
    ellipsis: true,
  },
  {
    colKey: 'uuid',
    title: 'GPU',
    width: 236,
    ellipsis: true,
    cell: (_h, { row }) => (
      <TTooltip content={row.uuid}>
        <div class="node-link-container" onClick={() => handleGpuJump(row.uuid)}>
          <div class="text">{row.uuid || '--'}</div>
          <svg-icon icon="jump" class="related-gpu-link-icon" />
        </div>
      </TTooltip>
    ),
  },
]);
const extractImageList = (payload) => {
  const all = (Array.isArray(payload?.images) ? payload.images : [])
    .map((item) => (typeof item === 'string' ? item.trim() : ''))
    .filter(Boolean);

  return Array.from(new Set(all));
};

const basicImage = computed(() => {
  const imageList = extractImageList(detail.value);
  if (!imageList.length) return '--';
  if (imageList.length === 1) return imageList[0];
  return `${imageList[0]}...`;
});
const basicImageTooltip = computed(() => {
  const imageList = extractImageList(detail.value);
  if (imageList.length <= 1) return '';
  return imageList.join('\n');
});
const basicCreateTime = computed(() => (
  detail.value?.createTime ? timeParse(detail.value.createTime) : '--'
));
const resourceOverviewData = useInstantVector(
  [
    {
      key: 'gpuCards',
      query: `sum(hami_container_vgpu_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"})`,
    },
    {
      key: 'computeLimit',
      query: `sum(hami_container_vcore_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"})`,
    },
    {
      key: 'singleCardMemory',
      query:
        `sum(hami_container_vmemory_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}) ` +
        `/ clamp_min(sum(hami_container_vgpu_allocated{container_name="$container",pod_name=~"$pod",namespace_name="$namespace"}), 1) / 1024`,
    },
    {
      key: 'cpuLimit',
      query:
        `sum(kube_pod_container_resource_limits{resource="cpu", namespace="$namespace", pod=~"$pod", container="$container"})`,
    },
    {
      key: 'memoryLimit',
      query:
        `sum(kube_pod_container_resource_limits{resource="memory", namespace="$namespace", pod=~"$pod", container="$container"}) / 1024 / 1024 / 1024`,
    },
  ],
  (query) =>
    query
      .replaceAll(`$container`, detail.value.name || '')
      .replaceAll(`$namespace`, detail.value.namespace || '')
      .replaceAll(`$pod`, detail.value.appName || ''),
);
const toNumOrUndefined = (v) => {
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
};
const resourceOverviewTexts = computed(() => {
  const get = (key) => resourceOverviewData.value.find((item) => item.key === key)?.count;
  const gpuCards = toNumOrUndefined(get('gpuCards'));
  const computeLimit = toNumOrUndefined(get('computeLimit'));
  const singleCardMemory = toNumOrUndefined(get('singleCardMemory'));
  const cpuLimit = toNumOrUndefined(get('cpuLimit'));
  const memoryLimit = toNumOrUndefined(get('memoryLimit'));
  return {
    gpuCards: gpuCards === undefined ? '--' : `${Math.round(gpuCards)}`,
    computeLimit: computeLimit === undefined ? '--' : `${roundToDecimal(computeLimit / 100, 1)}`,
    singleCardMemory: singleCardMemory === undefined ? '--' : `${singleCardMemory.toFixed(1)} GiB`,
    cpuLimit: cpuLimit === undefined ? '--' : `${Math.round(cpuLimit)} Core`,
    memoryLimit: memoryLimit === undefined ? '--' : `${memoryLimit.toFixed(1)} GiB`,
  };
});

const handleNodeJump = () => {
  const nodeName = detail.value?.nodeName || detail.value?.node_name || '';
  if (!nodeUid.value || !nodeName) return;
  router.push(`/admin/vgpu/node/admin/${nodeUid.value}?nodeName=${nodeName}`);
};
const handleGpuJump = (uuid) => {
  if (!uuid) return;
  router.push(`/admin/vgpu/card/admin/${uuid}`);
};

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

const headerStatusDisplay = computed(() => getStatusDisplay(detail.value?.status));

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
  cardTypeById.value = (cards.list || []).reduce((acc, item) => {
    if (item?.uuid) acc[item.uuid] = item?.type || '--';
    if (item?.id) acc[item.id] = item?.type || '--';
    return acc;
  }, {});
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
  display: block;
}

.task-detail {
  width: 100%;

  .basic-info-cards {
    width: 100%;
    display: flex;
    gap: 8px;
    align-items: center;
    margin-top: 0;
    margin-bottom: 20px;
  }

  .basic-info-card {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    flex: 1;
    border-radius: 8px;
    padding: 15px 20px;
    background: #f5f7fa;
    box-shadow: none;
    min-width: 0;
    align-self: normal;
  }

  .basic-info-card-title {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 4px;
    color: #1d2b3a;
    font-size: 16px;
    font-weight: 500;
    line-height: 28px;
    min-height: 28px;
    min-width: 0;
  }

  .gpu-model-container {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: nowrap;
    overflow: hidden;
    width: 100%;
    min-width: 0;
  }

  .gpu-model-item {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
  }

  .gpu-count {
    color: #324558;
    font-size: 14px;
    font-weight: 500;
    line-height: 22px;
    white-space: nowrap;
  }

  .gpu-separator {
    width: 1px;
    height: 22px;
    background: #e4ebf1;
    flex-shrink: 0;
  }

  :deep(.t-tag) {
    flex-shrink: 0;
    margin: 0;
    font-size: 13px;
    font-weight: 500;
    line-height: 22px;
    height: auto;
    padding: 2px 5px;
    color: #324558;
    border-radius: 6px;
    background-color: #e4ebf1;
    border-color: #d5dee7;
    white-space: nowrap;
  }

  .basic-info-card-sub-title {
    color: #939ea9;
    font-size: 12px;
    line-height: 20px;
    white-space: nowrap;
  }

  .node-link {
    cursor: pointer;
  }

  .gpu-eye-icon {
    cursor: pointer;
    color: #697886;
    font-size: 16px;
  }

  .related-gpu-count-text {
    color: #324558;
    font-size: 16px;
    font-weight: 500;
    line-height: 28px;
  }

  .basic-info-summary {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
  }

  .summary-item {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
  }

  .summary-item-label {
    width: 120px;
    color: #939ea9;
    font-size: 14px;
    line-height: 24px;
    flex-shrink: 0;
  }

  .summary-item-value {
    color: #324558;
    font-size: 14px;
    line-height: 24px;
    min-width: 0;
  }

  &.is-en {
    .summary-item-label { width: 120px; }
  }
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

.basic-info-block :deep(.home-block-content),
.workload-overview :deep(.home-block-content) {
  padding-top: 20px;
}

.basic-info-block,
.workload-overview {
  box-shadow: none;
}

.workload-overview {
  margin-top: 16px;
  padding: 20px;

  .row {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-top: 0;
  }

  .row-card {
    display: flex;
    align-items: center;
    flex: 1;
    min-width: 0;
    gap: 8px;
    padding: 15px 20px;
    border-radius: 8px;
    border: 0;
    background-color: #f5f7fa;
    align-self: normal;
  }

  .row-card-content {
    display: flex;
    align-items: center;
    gap: 20px;
    width: 100%;
    min-width: 0;
  }

  .row-card-content-icon {
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 10px;
    border-radius: 8px;
    background: #fff;
    box-shadow:
      0 2px 8px 0 rgb(2 5 8 / 4%),
      0 6px 20px 0 rgb(2 5 8 / 8%);
    flex-shrink: 0;
  }

  .row-card-content-icon :deep(svg) {
    font-size: 20px;
  }

  .row-card-content-info {
    min-width: 0;
  }

  .row-card-title {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 16px;
    font-weight: 500;
    color: #324558;
    line-height: 28px;
    min-height: 28px;
  }

  .row-card-sub-title {
    color: #939ea9;
    font-size: 12px;
    line-height: 20px;
  }
}

.trend-chart {
  height: 250px;
  margin-top: 15px;
}

.trend-vchart {
  width: 100%;
  height: 100%;
}

.relative-gpu-tooltip-content {
  width: 400px;
  max-width: 50vw;
  max-height: 40vh;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 16px 12px;
}

.popup-title {
  color: #1d2b3a;
  font-size: 14px;
  font-weight: 500;
}

.relative-gpu-tooltip-content :deep(.node-link-container) {
  display: flex;
  align-items: center;
  cursor: pointer;
}

.relative-gpu-tooltip-content :deep(.node-link-container .text) {
  width: 236px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.relative-gpu-tooltip-content :deep(.t-table__header th) {
  background: #f5f7fa;
  color: #939ea9;
  font-size: 14px;
  font-weight: 400;
  border: none;
}

.relative-gpu-tooltip-content :deep(.t-table__header th:first-child) {
  border-top-left-radius: 8px;
  border-bottom-left-radius: 8px;
}

.relative-gpu-tooltip-content :deep(.t-table__header th:last-child) {
  border-top-right-radius: 8px;
  border-bottom-right-radius: 8px;
}

.relative-gpu-tooltip-content :deep(.t-table__body td) {
  border: none;
  color: #1d2b3a;
  font-size: 14px;
  font-weight: 400;
}

.related-gpu-link-icon {
  flex-shrink: 0;
  color: #697886;
  font-size: 14px;
}

</style>
