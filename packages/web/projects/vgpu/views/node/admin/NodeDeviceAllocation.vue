<template>
  <section ref="section" class="node-allocation home-block" aria-labelledby="node-allocation-title" :aria-busy="status === 'loading'">
    <header class="node-allocation__header">
      <h2 id="node-allocation-title">{{ t('node.allocation.title') }}</h2>
      <RefreshButton compact :label="t('node.allocation.refresh')" :refreshing="status === 'loading'" :disabled="status === 'loading'" @click="retry" />
    </header>

    <div v-if="status === 'loading' || status === 'idle'" class="node-allocation__grid">
      <div v-for="index in 4" :key="index" class="node-device node-device--loading" aria-hidden="true">
        <t-skeleton animation="gradient" :row-col="skeletonRows" />
      </div>
      <span class="node-allocation__sr-only" role="status">{{ t('common.loading') }}</span>
    </div>
    <div v-else-if="status === 'error'" class="node-allocation__state" role="status">
      <span>{{ t('node.allocation.loadFailed') }}</span>
      <t-button variant="text" theme="primary" @click="retry">{{ t('common.retry') }}</t-button>
    </div>
    <p v-else-if="!entries.length" class="node-allocation__state">{{ t('node.allocation.empty') }}</p>
    <template v-else>
      <div id="node-allocation-devices" class="node-allocation__grid" :class="{ 'is-single': entries.length === 1 }">
        <article
          v-for="entry in visibleEntries"
          :key="entry.device.uuid"
          class="node-device"
          :data-device-id="entry.device.uuid"
          tabindex="-1"
        >
          <DeviceSplit
            class="node-device__split"
            :device="entry.device"
            :containers="entry.containers"
            show-device
            stack-summary
            :show-holders="false"
            :show-shared-count="false"
            :show-chart="entry.allocationStatus === 'complete'"
            :show-summary="entry.allocationStatus === 'complete'"
            strict-values
          >
            <template v-if="entry.allocationStatus !== 'complete'" #chart>
              <div class="node-device__state" role="status">
                <dl v-if="knownAllocation(entry).length" class="node-device__known-values">
                  <div v-for="value in knownAllocation(entry)" :key="value.key">
                    <dt>{{ t(value.label) }}</dt><dd>{{ value.text }}</dd>
                  </div>
                </dl>
                <p v-if="profiles(entry).length" class="node-device__profiles">
                  <span v-for="profile in profiles(entry)" :key="profile">{{ profile }}</span>
                </p>
                <p>{{ t(`node.allocation.${entry.allocationStatus}`) }}</p>
              </div>
            </template>
            <template #footer>
              <span>{{ occupancyText(entry) }}</span>
              <button
                v-if="entry.containerCount > 0"
                type="button"
                class="node-device__occupancy"
                aria-haspopup="dialog"
                :aria-label="t('node.allocation.occupancyLink', { id: entry.device.uuid })"
                @click="openOccupancy(entry, $event)"
              >{{ t('node.allocation.viewOccupancy') }}<ChevronRightIcon aria-hidden="true" /></button>
            </template>
          </DeviceSplit>
        </article>
      </div>
      <div v-if="entries.length > 4" class="node-allocation__expansion">
        <button
          ref="expandButton"
          type="button"
          :aria-expanded="expanded"
          aria-controls="node-allocation-devices"
          @click="toggleExpanded"
        >
          {{ expanded ? t('node.allocation.collapse') : t('node.allocation.expand', { count: entries.length - 4 }, entries.length - 4) }}
          <ChevronDownIcon :class="{ 'is-expanded': expanded }" aria-hidden="true" />
        </button>
      </div>
      <span class="node-allocation__sr-only" aria-live="polite">{{ t('node.allocation.visibleCount', { visible: visibleEntries.length, total: entries.length }) }}</span>
    </template>

    <t-drawer
      :visible="Boolean(selected)"
      :header="t('node.allocation.occupancyTitle')"
      drawer-class-name="node-occupancy-drawer"
      :footer="false"
      :close-btn="true"
      size="min(560px, 100vw)"
      :close-on-overlay-click="true"
      :close-on-esc-keydown="true"
      :role="selected ? 'dialog' : undefined"
      :aria-label="selected ? t('node.allocation.occupancyTitle') : undefined"
      :aria-modal="selected ? 'true' : undefined"
      :aria-hidden="selected ? undefined : 'true'"
      :tabindex="selected ? 0 : -1"
      @keydown="onDialogKeydown"
      @close="closeOccupancy"
    >
      <template #closeBtn><button type="button" class="node-occupancy__close" :aria-label="t('node.allocation.closeOccupancy')"><CloseIcon aria-hidden="true" /></button></template>
      <div v-if="selected" ref="drawerContent" class="node-occupancy">
        <DeviceSplit
          :device="selected.device"
          :containers="selected.containers"
          show-device
          stack-summary
          :show-chart="selected.allocationStatus === 'complete'"
          :show-summary="selected.allocationStatus === 'complete'"
          :show-shared-count="false"
          strict-values
        >
          <template v-if="selected.allocationStatus !== 'complete'" #chart>
            <p class="node-occupancy__count">{{ occupancyText(selected) }}</p>
            <p class="node-occupancy__notice" role="status">{{ t(`node.allocation.${selected.allocationStatus}`) }}</p>
          </template>
        </DeviceSplit>
      </div>
    </t-drawer>
  </section>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronDownIcon, ChevronRightIcon, CloseIcon } from 'tdesign-icons-vue-next';
import RefreshButton from '@/components/RefreshButton.vue';
import DeviceSplit from '~/vgpu/components/DeviceSplit.vue';
import useNodeDeviceAllocation from './useNodeDeviceAllocation';
import { readAllocationNumber } from './node-device-allocation.mjs';

const props = defineProps({ node: { type: Object, default: null } });
const { t } = useI18n();
const { entries, status, retry } = useNodeDeviceAllocation(computed(() => props.node));
const expanded = ref(false);
const selected = ref(null);
const section = ref(null);
const expandButton = ref(null);
const drawerContent = ref(null);
let opener = null;
const visibleEntries = computed(() => expanded.value ? entries.value : entries.value.slice(0, 4));
const skeletonRows = [
  { width: '62%', height: '18px' },
  { width: '40%', height: '14px', margin: '8px 0 0' },
  { width: '100%', height: '62px', margin: '16px 0 0' },
  { width: '35%', height: '16px', margin: '12px 0 0' },
];
const numberText = (value) => String(Math.round(value * 10) / 10);
const memoryText = (mib) => `${numberText(mib / 1024)} GiB`;
const allocationsOf = (entry) => entry.containers.flatMap((container) =>
  container.devices.filter((allocation) => allocation?.id === entry.device.uuid));
const knownAllocation = (entry) => {
  if (entry.allocationStatus === 'unconfigured') return [];
  const memory = readAllocationNumber(entry.device.memoryUsed);
  const compute = readAllocationNumber(entry.device.coreUsed);
  const values = [];
  if (memory !== undefined) values.push({ key: 'memory', label: 'node.allocation.allocatedMemory', text: memoryText(memory) });
  if (entry.device.coreUsedKnown === true && compute !== undefined) {
    const occupied = readAllocationNumber(entry.device.vgpuUsed) !== 0 || entry.containerCount > 0;
    const unlimited = compute === 0 && occupied && allocationsOf(entry).some((allocation) =>
      allocation.allocationShape === 'soft' && allocation.allocatedCoresKnown === true
        && readAllocationNumber(allocation.allocatedCores) === 0);
    if (compute > 0 || !occupied || unlimited) {
      values.push({ key: 'compute', label: 'node.allocation.allocatedCompute', text: unlimited ? t('common.notLimited') : `${numberText(compute)}%` });
    }
  }
  return values;
};
const profiles = (entry) => [...new Set(allocationsOf(entry)
  .map((allocation) => allocation.template)
  .filter((name) => typeof name === 'string' && name.trim()))];
const occupancyText = (entry) => {
  if (entry.containerCount === undefined) return t('node.allocation.occupancyUnknown');
  if (entry.allocationStatus !== 'complete') return t('node.allocation.knownWorkloads', { count: entry.containerCount }, entry.containerCount);
  if (entry.containerCount) return t('node.allocation.workloads', { count: entry.containerCount }, entry.containerCount);
  return t('node.allocation.noAllocation');
};

const toggleExpanded = async (event) => {
  expanded.value = !expanded.value;
  await nextTick();
  if (expanded.value && event.detail === 0) {
    const card = section.value?.querySelectorAll('.node-device')[4];
    (card?.querySelector('a') || card)?.focus();
  } else if (!expanded.value) {
    expandButton.value?.focus({ preventScroll: true });
    const bounds = expandButton.value?.getBoundingClientRect();
    if (bounds && (bounds.top < 0 || bounds.bottom > window.innerHeight)) expandButton.value.scrollIntoView({ block: 'nearest' });
  }
};
const openOccupancy = async (entry, event) => {
  opener = event.currentTarget;
  selected.value = entry;
  await nextTick();
  drawerContent.value?.closest('[role="dialog"]')?.focus();
};
const closeOccupancy = async () => {
  selected.value = null;
  await nextTick();
  requestAnimationFrame(() => {
    if (!selected.value && opener?.isConnected) opener.focus({ preventScroll: true });
  });
};
const onDialogKeydown = (event) => {
  if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    closeOccupancy();
    return;
  }
  if (event.key !== 'Tab') return;
  const dialog = drawerContent.value?.closest('[role="dialog"]');
  if (!dialog) return;
  const focusable = [...dialog.querySelectorAll('button:not([disabled]), a[href], [tabindex="0"]')]
    .filter((element) => element.getClientRects().length > 0);
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog)) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first?.focus();
  }
};
watch(() => props.node?.uid, () => {
  expanded.value = false;
  selected.value = null;
  opener = null;
});
</script>

<style scoped lang="scss">
.node-allocation {
  container-type: inline-size;
  color: #1d2b3a;
  padding: 12px 16px 16px;
}
.node-allocation__header {
  display: flex;
  align-items: center;
  gap: 12px;
}
.node-allocation__header { justify-content: space-between; flex-wrap: wrap; margin-bottom: 8px; }
.node-allocation__header h2 { margin: 0; font-size: 16px; font-weight: 500; line-height: 28px; }
.node-allocation__grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.node-allocation__grid.is-single { grid-template-columns: minmax(0, 1fr); }
.node-device { min-width: 0; }
.node-device__split { height: 100%; }
.node-device--loading { padding: 16px 20px; border-radius: 8px; background: #f5f7fa; }
.node-device__state { display: flex; flex-direction: column; justify-content: center; gap: 8px; min-height: 62px; padding: 12px; border-radius: 8px; background: #fff; color: #697886; font-size: 12px; line-height: 20px; }
.node-device__state p { margin: 0; }
.node-device__known-values { display: grid; gap: 6px; margin: 0; }
.node-device__known-values > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.node-device__known-values dt { font-weight: 400; }
.node-device__known-values dd { margin: 0; color: #324558; font-variant-numeric: tabular-nums; white-space: nowrap; }
.node-device__profiles { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; }
.node-device__occupancy,
.node-allocation__expansion button { display: inline-flex; align-items: center; gap: 4px; padding: 0; border: 0; background: transparent; color: var(--td-brand-color); font: inherit; cursor: pointer; }
.node-device__occupancy svg { width: 14px; height: 14px; }
.node-allocation__expansion { display: flex; justify-content: center; margin-top: 16px; }
.node-allocation__expansion button { padding: 4px 8px; border-radius: 4px; font-size: 12px; line-height: 20px; }
.node-allocation__expansion svg { width: 14px; height: 14px; }
.node-allocation__expansion .is-expanded { transform: rotate(180deg); }
.node-allocation__state { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 12px; min-height: 120px; margin: 0; color: #697886; }
.node-occupancy__count { margin: 0; color: #697886; font-size: 12px; }
.node-occupancy__notice { color: var(--td-warning-color); margin-bottom: 16px; font-size: 12px; }
.node-occupancy__close { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: inherit; cursor: pointer; }
.node-device__occupancy:focus-visible,
.node-allocation__expansion button:focus-visible,
.node-occupancy__close:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: 3px; border-radius: 3px; }
.node-allocation__sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }
@container (max-width: 680px) {
  .node-allocation__grid { grid-template-columns: minmax(0, 1fr); }
}
@container (max-width: 280px) {
  .node-device__known-values > div { align-items: flex-start; flex-direction: column; gap: 2px; }
  .node-device__occupancy { max-width: 100%; }
}
</style>

<style lang="scss">
.node-occupancy-drawer .t-drawer__content-wrapper--right {
  border-radius: 12px 0 0 12px;

  @media (max-width: 560px) {
    border-radius: 0;
  }
}
</style>
