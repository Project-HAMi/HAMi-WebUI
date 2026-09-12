<template>
  <span class="workload-status" :data-workload-status="status.code">
    <svg-icon :icon="statusIcon" class="workload-status__icon" aria-hidden="true" />
    <span class="workload-status__label">{{ status.label }}</span>
    <MetricHelp
      v-if="status.hasDetails"
      :description="status.description"
      :help-label="$t('task.statusHelpLabel')"
      multiline
    />
  </span>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import { getWorkloadStatus } from './workload-status.mjs';

const props = defineProps({ workload: { type: Object, default: () => ({}) } });
const { t } = useI18n();
const status = computed(() => getWorkloadStatus(props.workload, t));
const statusIcons = {
  success: 'status-schedulable',
  closed: 'status-schedulable',
  not_ready: 'status-unschedulable',
  failed: 'status-unschedulable',
  error: 'status-unschedulable',
};
const statusIcon = computed(() => statusIcons[status.value.code] || 'status-unmanaged');
</script>

<style scoped lang="scss">
.workload-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  line-height: 24px;
  white-space: nowrap;

  &__icon {
    flex: none;
    font-size: 16px;
  }

  &__label {
    align-self: baseline;
  }
}
</style>
