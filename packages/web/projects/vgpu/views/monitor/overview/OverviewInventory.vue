<template>
  <div class="home-inventory">
    <ResourceOverview
      :title="$t('dashboard.resourceOverview')"
      :resources="resourceSummaries"
      :labels="resourceSummaryLabels"
    />
    <Block :title="$t('dashboard.cardTypeDist')" class="model-summary" all-to="/accelerators" :all-label="$t('dashboard.viewAllGpuModels')">
      <div
        v-if="inventory.status === 'loading'"
        class="card-type-skeleton"
        aria-busy="true"
      >
        <t-skeleton
          animation="gradient"
          :row-col="Array.from({ length: 6 }, () => ({ width: '100%', height: '28px' }))"
          aria-hidden="true"
        />
        <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
      </div>
      <div
        v-else-if="inventory.status !== 'ready'"
        class="overview-state"
      >
        {{ t(stateTextKey(inventory.status, { metric: false })) }}
      </div>
      <div v-else-if="!modelRows.length" class="overview-state">
        {{ modelInventory?.partial ? $t('card.physicalInventoryUnavailable') : $t('common.noData') }}
      </div>
      <ModelDistribution
        v-else
        :models="modelRows"
        :partial="modelInventory.partial"
        :labels="modelDistributionLabels"
      />
    </Block>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { stateTextKey } from '~/vgpu/metrics/metric-state.mjs';
import Block from './Block.vue';
import ModelDistribution from './ModelDistribution.vue';
import ResourceOverview from './ResourceOverview.vue';

const props = defineProps({
  inventory: { type: Object, required: true },
  workloads: { type: Object, required: true },
});
const { t } = useI18n();
const modelInventory = computed(() => props.inventory.data?.models.inventory);
const modelRows = computed(() => props.inventory.data?.models.rows ?? []);

const modelDistributionLabels = computed(() => ({
  model: t('dashboard.modelDistributionColumns.model'),
  count: t('dashboard.modelDistributionColumns.count'),
  share: t('dashboard.modelDistributionColumns.share'),
  unknown: t('dashboard.unknownModel'),
  unit: t('common.unitSheet'),
  listLabel: t('dashboard.cardTypeDist'),
  partial: t('dashboard.modelDistributionPartial'),
}));

const resourceSummaryLabels = computed(() => ({
  autoRotate: t('dashboard.resourceSummary.autoRotate'),
  chooseResource: t('dashboard.resourceSummary.chooseResource'),
  loading: t('common.loading'),
  missing: t('dashboard.resourceSummary.missingStatus'),
  invalid: t('common.requestError'),
  error: t('common.requestError'),
  helpLabel: t('dashboard.resourceSummary.statusHelp'),
}));

const resourceSummaries = computed(() => {
  const inventory = props.inventory.data?.summary;
  const nodes = inventory?.nodes;
  const devices = inventory?.devices;
  const workloads = props.workloads.data;
  const status = props.inventory.status;
  return [
    {
      id: 'nodes', title: t('dashboard.nodeCount'), count: nodes?.count,
      status, panelStatus: status,
      panelTitle: t('dashboard.nodeStatus'), allTo: '/nodes', allLabel: t('dashboard.viewAllNodes'),
      description: t('node.temporarilyUnschedulableOverviewDescription'),
      items: [
        { id: 'schedulable', label: t('node.schedulable'), count: nodes?.schedulable, tone: 'success',
          to: { path: '/nodes', query: { schedulingEligibility: 'schedulable' } } },
        { id: 'temporarilyUnschedulable', label: t('node.temporarilyUnschedulable'), count: nodes?.unavailable, tone: 'neutral',
          to: { path: '/nodes', query: { schedulingEligibility: 'temporarilyUnschedulable' } } },
      ],
    },
    {
      id: 'gpus', title: t('dashboard.gpuCardCount'), count: devices?.count,
      status, panelStatus: status,
      panelTitle: t('card.status'), allTo: '/accelerators', allLabel: t('dashboard.viewAllGpus'),
      description: t('dashboard.resourceSummary.gpuStatusHelp'),
      items: [
        { id: 'healthy', label: t('card.normal'), count: devices?.counts.healthy, tone: 'success' },
        { id: 'unhealthy', label: t('card.abnormal'), count: devices?.counts.unhealthy, tone: devices?.counts.unhealthy > 0 ? 'danger' : 'neutral' },
        ...(devices?.counts.unknown > 0 ? [{ id: 'unknown', label: t('dashboard.resourceSummary.healthUnknown'), count: devices.counts.unknown, tone: 'neutral' }] : []),
      ],
    },
    {
      id: 'workloads', title: t('dashboard.workloadCount'), count: workloads?.total,
      status: props.workloads.status, panelStatus: props.workloads.status === 'ready' ? workloads.status : props.workloads.status,
      panelTitle: t('dashboard.resourceSummary.workloadStatus'), allTo: '/workloads', allLabel: t('dashboard.resourceSummary.viewAllWorkloads'),
      description: t('dashboard.resourceSummary.workloadStatusHelp'),
      items: [
        { id: 'running', label: t('task.statusRunning'), count: workloads?.counts?.success, tone: 'success', to: { path: '/workloads', query: { status: 'success' } } },
        { id: 'abnormal', label: t('task.statusAbnormal'), count: workloads?.counts?.abnormal, tone: workloads?.counts?.abnormal > 0 ? 'danger' : 'neutral', to: { path: '/workloads', query: { status: 'abnormal' } } },
        { id: 'other', label: t('dashboard.resourceSummary.otherStatus'), count: workloads?.counts?.other, tone: 'neutral' },
      ],
    },
  ];
});
</script>
