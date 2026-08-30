<template>
  <page-header
    :title="$t('task.workload')"
    :name="workloadDisplayName"
    :status="headerStatusDisplay.text"
    :status-icon="headerStatusDisplay.icon"
  >
    <template #titleSuffix>
      <ElPopover
        v-if="
          detailStatus === REQUEST_STATUS.READY &&
          (detail.status === 'unknown' || detail.status === 'failed')
        "
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

  <detail-page-state :status="detailStatus" @retry="retryDetail">

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
              <span class="summary-item-label">{{ $t('task.detail.podName') }}</span>
              <span class="summary-item-value">
                <ellipsis-text :text="detail.appName || '--'" mode="middle" tooltip="always" />
              </span>
            </div>
            <div class="summary-item">
              <span class="summary-item-label">{{ $t('task.detail.containerName') }}</span>
              <span class="summary-item-value">
                <ellipsis-text :text="detail.name || '--'" mode="middle" tooltip="always" />
              </span>
            </div>
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
    v-if="supportsTaskMonitoring"
    v-model="times"
  />

  <div class="task-trend-row">
    <block-box v-for="{ title, data } in lineConfigView" :key="title" :title="title">
      <div class="trend-chart">
        <template v-if="primaryDeviceType && !supportsTaskMonitoring">
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
  </detail-page-state>
</template>

<script setup lang="jsx">
import PageHeader from '@/components/PageHeader.vue';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import { useRoute, useRouter } from 'vue-router';
import { ref, watch, computed } from 'vue';
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
import { formatWorkloadName } from './workload-identity.mjs';
import { METRIC_STATUS } from '~/vgpu/hooks/instant-vector-state.mjs';
import DetailPageState from '~/vgpu/components/DetailPageState.vue';
import { classifyDetailPayload } from '~/vgpu/hooks/detail-resource-state.mjs';
import useDetailResource from '~/vgpu/hooks/useDetailResource';

const route = useRoute();
const router = useRouter();
const { t, locale } = useI18n();

const normalizeRouteQuery = (value) => {
  const normalized = Array.isArray(value) ? value[0] : value;
  return typeof normalized === 'string' ? normalized.trim() : '';
};
const requestedIdentity = computed(() => ({
  name: normalizeRouteQuery(route.query.name),
  podUid: normalizeRouteQuery(route.query.podUid),
}));
const {
  data: detail,
  status: detailStatus,
  retry: retryDetail,
} = useDetailResource({
  source: requestedIdentity,
  isValidSource: ({ name, podUid } = {}) => Boolean(name && podUid),
  request: ({ name, podUid }) => taskApi.getTaskDetail({ name, podUid }),
  classify: (payload, identity) => classifyDetailPayload(payload, {
    identityKeys: ['name', 'podUid'],
    expectedIdentity: identity,
  }),
});
const workloadDisplayName = computed(() => (
  detailStatus.value === REQUEST_STATUS.READY
    ? formatWorkloadName(detail.value)
    : requestedIdentity.value.name || '--'
));
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
const primaryDeviceType = computed(() => {
  const primaryDeviceId = safeDeviceIds.value[0];
  return (
    (primaryDeviceId && cardTypeById.value?.[primaryDeviceId]) ||
    detail.value?.type ||
    ''
  );
});
const supportsTaskMonitoring = computed(() => (
  primaryDeviceType.value.startsWith('NVIDIA') ||
  primaryDeviceType.value.startsWith('MXC')
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
    {
      key: 'containerInfo',
      query:
        `count(kube_pod_container_info{namespace="$namespace", pod=~"$pod", container="$container"})`,
    },
  ],
  (query) => {
    if (detailStatus.value !== REQUEST_STATUS.READY) return 'undefined';
    return query
      .replaceAll(`$container`, detail.value.name || '')
      .replaceAll(`$namespace`, detail.value.namespace || '')
      .replaceAll(`$pod`, detail.value.appName || '');
  },
);
const toNumOrUndefined = (v) => {
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
};
const resourceOverviewTexts = computed(() => {
  const getMetric = (key) => resourceOverviewData.value.find((item) => item.key === key);
  const get = (key) => getMetric(key)?.count;
  const containerInfo = getMetric('containerInfo');
  const hasContainerInfo =
    containerInfo?.hasData === true && Number(containerInfo.count) > 0;
  const formatLimit = (key, formatter) => {
    const metric = getMetric(key);
    if (metric?.hasData === true) {
      const value = toNumOrUndefined(metric.count);
      return value === undefined ? '--' : formatter(value);
    }
    if (metric?.status === METRIC_STATUS.MISSING && hasContainerInfo) {
      return t('common.notLimited');
    }
    return '--';
  };
  const gpuCards = toNumOrUndefined(get('gpuCards'));
  const computeLimit = toNumOrUndefined(get('computeLimit'));
  const singleCardMemory = toNumOrUndefined(get('singleCardMemory'));
  return {
    gpuCards: gpuCards === undefined ? '--' : `${Math.round(gpuCards)}`,
    computeLimit: computeLimit === undefined ? '--' : `${roundToDecimal(computeLimit / 100, 1)}`,
    singleCardMemory: singleCardMemory === undefined ? '--' : `${singleCardMemory.toFixed(1)} GiB`,
    cpuLimit: formatLimit('cpuLimit', (value) => `${roundToDecimal(value, 3)} Core`),
    memoryLimit: formatLimit('memoryLimit', (value) => `${value.toFixed(1)} GiB`),
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
    query: `avg(avg(hami_container_core_util{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (node, device_uuid))`,
    data: [],
  },
  {
    titleKey: 'task.memUsageTrend',
    query: `100 * sum(avg(hami_container_memory_used{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (node, device_uuid)) / clamp_min(sum(avg(hami_container_vmemory_allocated{container_name=~"$container",pod_name=~"$pod",namespace_name="$namespace"}) by (node, device_uuid)), 1)`,
    data: [],
  },
]);

const headerStatusDisplay = computed(() => (
  detailStatus.value === REQUEST_STATUS.READY
    ? getStatusDisplay(detail.value?.status)
    : { text: '', icon: '' }
));

const lineConfigView = computed(() =>
  lineConfig.value.map((item) => ({
    ...item,
    title: t(item.titleKey),
  })),
);

let lineRequestGeneration = 0;
const clearLineData = () => {
  lineConfig.value.forEach((item) => {
    item.data = [];
  });
};
const fetchLineData = async () => {
  const generation = ++lineRequestGeneration;
  if (
    detailStatus.value !== REQUEST_STATUS.READY ||
    !supportsTaskMonitoring.value
  ) {
    clearLineData();
    return;
  }

  const { name, namespace, appName } = detail.value;
  const [rangeStart, rangeEnd] = times.value || [];
  if (!name || !namespace || !appName || !rangeStart || !rangeEnd) {
    clearLineData();
    return;
  }

  const results = await Promise.all(lineConfig.value.map(async (item) => {
    try {
      const res = await cardApi.getRangeVector({
        range: {
          start: timeParse(rangeStart),
          end: timeParse(rangeEnd),
          step: calculatePrometheusStep(rangeStart, rangeEnd),
        },
        query: item.query
          .replaceAll(`$container`, name)
          .replaceAll(`$namespace`, namespace)
          .replaceAll(`$pod`, appName),
      });
      return { ok: true, data: res.data[0]?.values || [] };
    } catch {
      return { ok: false };
    }
  }));

  if (
    generation !== lineRequestGeneration ||
    detailStatus.value !== REQUEST_STATUS.READY
  ) return;

  results.forEach((result, index) => {
    if (result.ok) lineConfig.value[index].data = result.data;
  });
};

watch(
  [detailStatus, detail, times, primaryDeviceType],
  () => {
    void fetchLineData();
  },
  { deep: true, immediate: true },
);

let enrichmentGeneration = 0;
watch(
  [detailStatus, detail],
  ([status, currentDetail]) => {
    const generation = ++enrichmentGeneration;
    nodeUid.value = '';
    cardTypeById.value = {};
    if (status !== REQUEST_STATUS.READY) return;

    const isCurrent = () => (
      generation === enrichmentGeneration &&
      detailStatus.value === REQUEST_STATUS.READY &&
      detail.value?.name === currentDetail?.name &&
      detail.value?.podUid === currentDetail?.podUid
    );

    nodeUid.value = currentDetail.nodeUid || currentDetail.node_uid || '';

    void (async () => {
      try {
        const cards = await cardApi.getCardListReq({ filters: {} });
        if (!isCurrent()) return;
        cardTypeById.value = (cards.list || []).reduce((acc, item) => {
          if (item?.uuid && item?.type) acc[item.uuid] = item.type;
          if (item?.id && item?.type) acc[item.id] = item.type;
          return acc;
        }, {});
      } catch {
        // Device enrichment is optional; keep the core detail usable.
      }
    })();

    const nodeName = currentDetail.nodeName || currentDetail.node_name || '';
    if (!nodeUid.value && nodeName) {
      void (async () => {
        try {
          const { list = [] } = await nodeApi.getNodes({ filters: {} });
          if (!isCurrent()) return;
          const node = list.find((item) => item?.name === nodeName);
          nodeUid.value = node?.uid || '';
        } catch {
          // Node enrichment is optional; keep the core detail usable.
        }
      })();
    }
  },
  { deep: true, immediate: true },
);
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
