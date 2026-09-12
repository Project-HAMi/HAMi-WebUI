<template>
  <span class="ranking-workload">
    <RouterLink
      v-if="location"
      class="ranking-workload-link"
      :to="location"
      :aria-label="label"
      @click.stop
    >
      <span class="ranking-workload-label">
        <template v-if="workload.appName">
          <span v-if="workload.appName !== workload.identity.name" class="ranking-pod-name">
            <EllipsisText :text="workload.appName" mode="middle" tooltip="always" />
          </span>
          <span v-if="workload.appName !== workload.identity.name" class="ranking-separator" aria-hidden="true">/</span>
          <span class="ranking-container-name">
            <EllipsisText :text="workload.identity.name" tooltip="overflow" />
          </span>
        </template>
        <template v-else>
          <span class="ranking-container-name">
            <EllipsisText :text="workload.identity.name" tooltip="overflow" />
          </span>
          <span class="ranking-separator" aria-hidden="true">/</span>
          <span class="ranking-pod-uid">
            Pod UID <EllipsisText :text="workload.identity.podUid" mode="middle" :head="8" :tail="4" tooltip="always" />
          </span>
        </template>
      </span>
    </RouterLink>
    <EllipsisText v-else :text="label" tooltip="always" />
    <span class="ranking-separator" aria-hidden="true">|</span>
    <span class="ranking-namespace" :aria-label="`${$t('task.namespace')}: ${workload.namespace || '--'}`">
      <EllipsisText :text="workload.namespace || '--'" tooltip="overflow" />
    </span>
  </span>
</template>

<script setup>
import { computed } from 'vue';
import { RouterLink } from 'vue-router';
import EllipsisText from '@/components/EllipsisText.vue';
import { createWorkloadDetailLocation, formatWorkloadName } from './workload-identity.mjs';

const props = defineProps({ workload: { type: Object, required: true } });
const location = computed(() => createWorkloadDetailLocation(props.workload.metricName));
const label = computed(() => {
  const { identity, appName, metricName } = props.workload;
  if (!identity) return metricName || '--';
  return appName
    ? formatWorkloadName({ appName, name: identity.name })
    : `${identity.name} / Pod UID ${identity.podUid}`;
});
</script>

<style scoped lang="scss">
.ranking-workload {
  display: flex;
  flex: 1;
  align-items: baseline;
  gap: 8px;
  line-height: 20px;
  min-width: 0;
}

.ranking-workload-link {
  display: flex;
  min-width: 0;
  color: #324558;
  font-size: 13px;
  text-decoration: none;

  &:hover,
  &:focus-visible {
    color: var(--el-color-primary);
  }

  &:focus-visible {
    border-radius: 4px;
    outline: 2px solid #2563eb;
    outline-offset: 2px;
  }
}

.ranking-workload-label {
  position: relative;
  display: inline-flex;
  flex: 0 1 auto;
  align-items: baseline;
  gap: 6px;
  max-width: 100%;
  min-width: 0;
  line-height: inherit;

  &::after {
    position: absolute;
    right: 0;
    bottom: 0;
    left: 0;
    height: 1px;
    background: currentcolor;
    content: '';
    opacity: 0;
    pointer-events: none;
  }
}

.ranking-workload-link:hover .ranking-workload-label::after,
.ranking-workload-link:focus-visible .ranking-workload-label::after {
  opacity: 1;
}

.ranking-workload-label .ranking-separator {
  color: inherit;
}

.ranking-pod-name {
  display: flex;
  overflow: hidden;
  line-height: inherit;
  flex: 0 1 auto;
  min-width: 0;
}

.ranking-container-name {
  display: flex;
  overflow: hidden;
  line-height: inherit;
  flex: 0 1 auto;
  min-width: 0;
}

.ranking-separator {
  flex-shrink: 0;
  color: #939ea9;
}

.ranking-pod-uid,
.ranking-namespace {
  display: flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
  color: #939ea9;
  font-size: 13px;
}

.ranking-pod-uid {
  white-space: nowrap;
}

.ranking-namespace {
  max-width: 45%;
}

.ranking-workload :deep(.ellipsis-text) {
  max-width: 100%;
  min-width: 0;
  vertical-align: bottom;
}
</style>
