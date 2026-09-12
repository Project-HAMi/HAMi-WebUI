<template>
  <span class="workload-status" :data-workload-status="status.code">
    <span class="workload-status__label">{{ status.label }}</span>
    <MetricHelp
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
</script>

<style scoped lang="scss">
.workload-status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  line-height: 24px;
  white-space: nowrap;
  color: #526477;

  &[data-workload-status='success'] {
    color: #15803d;
  }

  &[data-workload-status='waiting'],
  &[data-workload-status='not_ready'] {
    color: #a16207;
  }

  &[data-workload-status='error'],
  &[data-workload-status='failed'] {
    color: #b91c1c;
  }
}
</style>
