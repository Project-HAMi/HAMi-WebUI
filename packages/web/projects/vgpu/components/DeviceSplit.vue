<template>
  <section class="device-split" :aria-busy="status === 'loading'">
    <header class="device-split__head" :class="{ 'has-device': showDevice }">
      <div v-if="showDevice" class="device-split__device">
        <span class="device-split__icon" aria-hidden="true">
          <svg-icon :icon="deviceIcon" />
        </span>
        <div class="device-split__identity">
          <span class="device-split__model">
            {{ device.type || '--' }}
            <t-tag v-if="modeText" size="small" theme="primary" variant="light">{{ modeText }}</t-tag>
          </span>
          <RouterLink class="device-split__uuid" :to="`/accelerators/${device.uuid}`" :title="device.uuid">
            {{ device.uuid }}
          </RouterLink>
        </div>
      </div>
      <p v-if="status === 'ready'" class="device-split__summary">
        <span class="device-split__items">
          <span
            v-for="(part, i) in summary"
            :key="i"
            class="device-split__item"
            :class="{ 'is-warning': part.warning }"
          >{{ part.text }}</span>
          <span v-if="hasStranded" class="device-split__item device-split__legend">
            <i class="device-split__legend-mark" aria-hidden="true" />{{ $t('card.split.strandedLegend') }}
          </span>
        </span>
      </p>
    </header>

    <template v-if="status === 'loading'">
      <t-skeleton animation="gradient" :row-col="skeletonRows" aria-hidden="true" />
      <span class="device-split__sr-only" role="status">{{ $t('common.loading') }}</span>
    </template>

    <div v-else-if="status === 'error'" class="device-split__state" role="status">
      <span>{{ $t('card.split.loadFailed') }}</span>
      <t-button size="small" variant="text" theme="primary" @click="emit('retry')">{{ $t('common.retry') }}</t-button>
    </div>

    <template v-else>
      <div v-if="split.kind === 'mig' && split.slots" class="split-mig" aria-hidden="true">
        <div class="split-mig__grid" :style="{ '--slots': split.slots, '--lanes': split.lanes }">
          <span
            v-for="cell in openCells"
            :key="`cell-${cell.slot}`"
            class="split-mig__cell"
            :class="`is-${cell.state}`"
            :style="{ gridColumn: cell.slot + 1 }"
          />
          <span
            v-for="block in split.blocks"
            :key="block.key"
            class="split-part"
            :class="partClass(block)"
            :style="{ gridColumn: `${block.placement.start + 1} / span ${block.placement.size}`, gridRow: block.lane + 1 }"
            @mouseenter="active = block.key"
            @mouseleave="active = ''"
          >
            <span class="split-part__label">{{ labelOf(block) }}</span>
          </span>
        </div>
        <div class="split-mig__ruler" :style="{ '--slots': split.slots }">
          <span v-for="slot in split.slots" :key="slot">{{ slot - 1 }}</span>
        </div>
      </div>

      <div v-else-if="split.kind === 'shared'" class="split-meters">
        <div v-for="row in meters" :key="row.key" class="split-meter">
          <span class="split-meter__name">{{ row.name }}</span>
          <div class="split-meter__track" aria-hidden="true">
            <span
              v-for="part in row.meter.parts"
              :key="part.key"
              class="split-meter__part"
              :class="{ 'is-current': part.current, 'is-active': active === part.key }"
              :style="{ width: `${part.share * 100}%` }"
            />
          </div>
          <span class="split-meter__value" :class="{ 'is-warning': row.meter.over }">{{ row.value }}</span>
        </div>
      </div>

      <div
        v-else-if="split.kind === 'memory'"
        class="split-strip"
        aria-hidden="true"
        :style="{ '--gaps': `${stripGaps}px` }"
      >
        <span
          v-for="block in split.blocks"
          :key="block.key"
          class="split-part"
          :class="partClass(block)"
          :style="{ width: stripWidth(block.share) }"
          @mouseenter="active = block.key"
          @mouseleave="active = ''"
        >
          <span class="split-part__label">{{ labelOf(block) }}</span>
        </span>
        <span
          v-if="split.free"
          class="split-strip__rest"
          :style="{ width: stripWidth(split.free / Math.max(split.total, split.used)) }"
        >
          <span class="split-part__label">{{ $t('card.split.unallocated', { size: memoryText(split.free) }) }}</span>
        </span>
      </div>

      <ul v-if="split.holders.length" class="split-rows" :class="`is-${split.kind}`">
        <li
          v-for="holder in split.holders"
          :key="holder.key"
          class="split-row"
          :class="{ 'is-current': holder.current, 'is-active': active === holder.key }"
          @mouseenter="active = holder.key"
          @mouseleave="active = ''"
        >
          <span class="split-row__swatch" :class="swatchClass(holder)" aria-hidden="true" />
          <span v-if="split.kind === 'mig'" class="split-row__slot">{{ placementText(holder) }}</span>
          <span v-if="split.kind !== 'shared'" class="split-row__part">
            {{ labelOf(holder) }}
            <MetricHelp
              v-if="reasonOf(holder)"
              multiline
              :description="reasonOf(holder)"
              :help-label="$t('task.allocation.shapeReasonLabel')"
            />
          </span>
          <span class="split-row__memory">{{ memoryText(holder.memoryMiB) }}</span>
          <span class="split-row__compute">{{ computeLabel(holder) }}</span>
          <span class="split-row__holder">
            <span v-if="holder.current">{{ workloadText(holder.workload) }}</span>
            <RouterLink
              v-else-if="locationOf(holder)"
              class="split-row__link"
              :to="locationOf(holder)"
              @focus="active = holder.key"
              @blur="active = ''"
            >{{ workloadText(holder.workload) }}</RouterLink>
            <span v-else>{{ workloadText(holder.workload) }}</span>
            <span v-if="holder.current" class="split-row__self">{{ $t('card.split.thisWorkload') }}</span>
          </span>
        </li>
      </ul>
      <p v-else class="device-split__empty">{{ $t('card.split.empty') }}</p>
    </template>
  </section>
</template>

<script setup>
import { computed, ref } from 'vue';
import { RouterLink } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { buildDeviceSplit } from './device-split.mjs';
import { getSplitIcon, getSplitModeKey } from './split-mode.mjs';
import MetricHelp from './MetricHelp.vue';
import { getShapeUnknownReasonKey } from '~/vgpu/views/task/admin/allocation-display.mjs';
import { buildWorkloadDetailLocation, formatWorkloadName } from '~/vgpu/views/task/admin/workload-identity.mjs';

const props = defineProps({
  device: { type: Object, required: true },
  containers: { type: Array, default: () => [] },
  highlight: { type: Object, default: undefined },
  status: { type: String, default: 'ready' },
  showDevice: { type: Boolean, default: false },
});
const emit = defineEmits(['retry']);
const { t } = useI18n();
const active = ref('');

const split = computed(() => buildDeviceSplit({ device: props.device, containers: props.containers, highlight: props.highlight }));
const openCells = computed(() => (split.value.cells || []).filter((cell) => cell.state !== 'used'));
const modeText = computed(() => {
  const key = getSplitModeKey(props.device.mode);
  return key ? t(key) : '';
});
const deviceIcon = computed(() => getSplitIcon(props.device.mode) || 'vgpu-card');
const skeletonRows = [
  { width: '100%', height: '36px' },
  { width: '100%', height: '32px', margin: '12px 0 0' },
];

const trimmed = (value) => String(Math.round(value * 10) / 10);
const memoryText = (mib) => `${trimmed(Number(mib || 0) / 1024)} GiB`;
const computeText = (holder) => {
  if (!holder.coresKnown) return t('card.split.computeUnknown');
  return holder.cores ? `${trimmed(holder.cores)}%` : t('common.notLimited');
};
const labelOf = (holder) => {
  if (holder.shape === 'mig') return holder.name || t('task.allocation.shape.mig');
  if (holder.shape === 'template') return holder.name || t('task.allocation.shape.template');
  if (holder.shape === 'whole') return t('task.allocation.shape.whole');
  if (holder.shape === 'soft') return t('task.allocation.shape.soft');
  if (holder.shape === 'unknown') return t('task.allocation.shape.unknown');
  return '--';
};
const reasonOf = (holder) => {
  const key = getShapeUnknownReasonKey({
    allocationShape: holder.shape,
    allocationShapeReason: holder.shapeReason,
    allocatedCoresKnown: holder.coresKnown,
    allocatedCoresReason: holder.coresReason,
  });
  return key ? t(key) : '';
};
const slotText = ({ start, size }) => (size > 1
  ? t('card.split.slices', { start, end: start + size - 1 })
  : t('card.split.slice', { start }));
const computeLabel = (holder) => `${t('card.split.compute')} ${computeText(holder)}`;
const joinParts = (parts) => parts.filter(Boolean).join(t('card.split.separator'));
const placementText = (holder) => (split.value.kind === 'mig' && holder.placement ? slotText(holder.placement) : '');
const workloadText = (workload) => {
  const name = formatWorkloadName({ appName: workload.appName, name: workload.container });
  return workload.namespace ? `${workload.namespace} / ${name}` : name;
};
const hasStranded = computed(() => (split.value.cells || []).some((cell) => cell.state === 'stranded'));
const stripGaps = computed(() => {
  const parts = (split.value.blocks || []).length + (split.value.free ? 1 : 0);
  return Math.max(parts - 1, 0) * 4;
});
const stripWidth = (share) => `calc((100% - var(--gaps)) * ${share})`;
const locationOf = (holder) => buildWorkloadDetailLocation({ podUid: holder.workload.podUid, name: holder.workload.container });
const partClass = (holder) => ({
  'is-current': holder.current,
  'is-active': active.value === holder.key,
  'is-unknown': holder.shape === 'unknown',
});
const swatchClass = (holder) => ({
  'is-current': holder.current,
  'is-unknown': holder.shape === 'unknown',
});

const meters = computed(() => {
  const { memory, compute, unlimited } = split.value;
  if (split.value.kind !== 'shared') return [];
  const memoryValue = memory.total
    ? `${memoryText(memory.used)} / ${memoryText(memory.total)}`
    : joinParts([memoryText(memory.used), t('card.split.memoryUnknown')]);
  const computeValue = [`${trimmed(compute.used)}% / ${trimmed(compute.total)}%`];
  if (unlimited) computeValue.push(t('card.split.unlimitedCount', { count: unlimited }));
  return [
    { key: 'memory', name: t('card.split.memory'), meter: memory, value: memoryValue },
    { key: 'compute', name: t('card.split.compute'), meter: compute, value: joinParts(computeValue) },
  ];
});

const summary = computed(() => {
  const value = split.value;
  const parts = [];
  if (value.kind === 'mig') {
    parts.push({ text: t('card.split.instances', { count: value.blocks.length }, value.blocks.length) });
    // Each count is an alternative: what fits if only that profile is placed.
    if (value.fits?.length) {
      const list = value.fits.map((fit) => `${fit.name} ×${fit.count}`).join(t('card.split.listSeparator'));
      parts.push({ text: t('card.split.fits', { list }) });
    }
    if (value.overlapping) parts.push({ text: t('card.split.overlapping'), warning: true });
    if (value.beyondRegistered) parts.push({ text: t('card.split.beyondRegistered'), warning: true });
  } else if (value.kind === 'shared') {
    parts.push({ text: value.limit
      ? t('card.split.sharedBy', { count: value.holderCount, limit: value.limit })
      : t('card.split.sharedByCount', { count: value.holderCount }) });
    if (value.memory.over) parts.push({ text: t('card.split.memoryOver', { size: memoryText(value.memory.over) }), warning: true });
  } else {
    parts.push({ text: value.total
      ? t('card.split.memoryUsed', { used: memoryText(value.used), total: memoryText(value.total) })
      : t('card.split.memoryUnknown') });
    if (value.over) parts.push({ text: t('card.split.memoryOver', { size: memoryText(value.over) }), warning: true });
  }
  return parts;
});
</script>

<style lang="scss" scoped>
.device-split {
  --split-current: #2563eb;
  --split-current-active: #1d4ed8;
  --split-other: #dbe6fb;
  --split-other-active: #bfd3f8;
  --split-other-text: #1e3a8a;
  --split-line: #c8d3de;
  --split-track: #e5e7eb;

  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px 20px;
  border-radius: 8px;
  background: #f5f7fa;
}

.device-split__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 24px;

  &.has-device {
    justify-content: space-between;
  }
}

.device-split__device {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.device-split__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  flex-shrink: 0;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 2px 8px 0 rgb(2 5 8 / 4%), 0 6px 20px 0 rgb(2 5 8 / 8%);
  font-size: 18px;
}

.device-split__identity {
  display: flex;
  flex-direction: column;
  min-width: 0;
  line-height: 20px;
}

.device-split__model {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 8px;
  color: #1d2b3a;
  font-size: 14px;
  font-weight: 500;
}

.device-split__uuid {
  overflow: hidden;
  color: #697886;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-decoration: underline;
  text-decoration-color: transparent;
  text-overflow: ellipsis;
  text-underline-offset: 3px;
  white-space: nowrap;
  transition: color 0.2s ease, text-decoration-color 0.2s ease;

  &:hover,
  &:focus-visible {
    color: var(--split-current);
    text-decoration-color: currentColor;
  }
}

.device-split__summary {
  overflow: hidden;
  margin: 0;
  color: #697886;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  line-height: 20px;

  .is-warning {
    color: #d54941;
  }
}

// Every item carries a divider on its left; the one that starts a line is clipped.
.device-split__items {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  row-gap: 4px;
  margin-left: -32px;
}

.device-split__item {
  position: relative;
  margin-left: 16px;
  padding-left: 16px;

  &::before {
    position: absolute;
    top: 50%;
    left: 0;
    width: 1px;
    height: 12px;
    background: #dcdfe4;
    content: '';
    transform: translateY(-50%);
  }
}

.device-split__legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.device-split__legend-mark {
  width: 12px;
  height: 12px;
  border-radius: 3px;
  background: repeating-linear-gradient(135deg, #dfe5ec 0 2px, #fff 2px 4px);
  box-shadow: inset 0 0 0 1px #dfe5ec;
}

.split-part {
  display: flex;
  align-items: center;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  overflow: hidden;
  border-radius: 4px;
  background: var(--split-other);
  color: var(--split-other-text);
  font-size: 12px;
  font-weight: 500;
  transition: background-color 0.15s ease, box-shadow 0.15s ease;
  container-type: inline-size;

  box-shadow: inset 0 0 0 1px #b7cbf4;

  &.is-active {
    background: var(--split-other-active);
  }

  &.is-current {
    box-shadow: none;
    background: var(--split-current);
    color: #fff;

    &.is-active {
      background: var(--split-current-active);
    }
  }

  &.is-unknown:not(.is-current) {
    background: repeating-linear-gradient(135deg, var(--split-other) 0 6px, #eaf0fb 6px 12px);
  }
}

.split-part__label {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

// Too narrow to name the part; the row below still does.
@container (max-width: 44px) {
  .split-part__label {
    display: none;
  }
}

.split-mig__grid {
  display: grid;
  grid-template-columns: repeat(var(--slots), minmax(0, 1fr));
  grid-template-rows: repeat(var(--lanes), 32px);
  gap: 4px;
  padding: 6px;
  border-radius: 10px;
  background: #fff;
}

.split-mig__cell {
  grid-row: 1 / -1;
  border-radius: 4px;

  &.is-open {
    border: 1px dashed var(--split-line);
  }

  &.is-stranded {
    background: repeating-linear-gradient(135deg, #eef2f6 0 4px, #fff 4px 8px);
  }

  &.is-unknown {
    background: #f2f4f7;
  }
}

.split-mig__ruler {
  display: grid;
  grid-template-columns: repeat(var(--slots), minmax(0, 1fr));
  gap: 4px;
  margin-top: 4px;
  padding: 0 6px;
  color: #939ea9;
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  line-height: 14px;
  text-align: center;
}

.split-strip {
  display: flex;
  gap: 4px;
  padding: 6px;
  border-radius: 10px;
  background: #fff;

  .split-part {
    flex: 0 0 auto;
  }
}

.split-strip__rest {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  border: 1px dashed var(--split-line);
  border-radius: 4px;
  color: #939ea9;
  font-size: 12px;
  container-type: inline-size;
}

.split-meters {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr) max-content;
  align-items: center;
  gap: 10px 16px;
  padding: 12px 16px;
  border-radius: 8px;
  background: #fff;
  font-size: 12px;
}

.split-meter {
  display: contents;
}

.split-meter__name {
  color: #697886;
}

.split-meter__track {
  display: flex;
  gap: 2px;
  height: 8px;
  overflow: hidden;
  border-radius: 4px;
  background: var(--split-track);
}

.split-meter__part {
  background: #93b4f2;
  transition: background-color 0.15s ease;

  &.is-active {
    background: #6f97e8;
  }

  &.is-current {
    background: var(--split-current);
  }
}

.split-meter__value {
  color: #324558;
  font-variant-numeric: tabular-nums;
  text-align: right;

  &.is-warning {
    color: #d54941;
  }
}

.split-rows {
  display: grid;
  grid-template-columns: 8px repeat(3, max-content) minmax(0, 1fr);
  gap: 6px 16px;
  margin: 0;
  padding: 0;
  list-style: none;

  &.is-mig {
    grid-template-columns: 8px repeat(4, max-content) minmax(0, 1fr);
  }

  &.is-shared {
    grid-template-columns: 8px repeat(2, max-content) minmax(0, 1fr);
  }
}

.split-row {
  display: grid;
  grid-column: 1 / -1;
  grid-template-columns: subgrid;
  align-items: center;
  min-height: 36px;
  padding: 0 12px;
  border-radius: 6px;
  background: #fff;
  box-shadow: inset 0 0 0 1px transparent;
  color: #697886;
  font-size: 12px;
  transition: background-color 0.15s ease, box-shadow 0.15s ease;

  &.is-active {
    background: #f4f8ff;
  }

  &.is-current {
    background: #f2f7ff;
    box-shadow: inset 0 0 0 1px #cddffb;

    .split-row__slot,
    .split-row__part,
    .split-row__holder {
      color: #1d2b3a;
    }

    &.is-active {
      background: #e9f1ff;
    }
  }
}

.split-row__swatch {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  background: var(--split-other-active);

  &.is-current {
    background: var(--split-current);
  }

  &.is-unknown:not(.is-current) {
    background: repeating-linear-gradient(135deg, var(--split-other-active) 0 2px, #fff 2px 4px);
  }
}

.split-row__slot {
  color: #324558;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.split-row__part {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  overflow: hidden;
  color: #1d2b3a;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.split-row__memory,
.split-row__compute {
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.split-row__memory {
  justify-self: end;
}

.split-row__holder {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  overflow: hidden;
  color: #324558;
  white-space: nowrap;

  > :first-child {
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

.split-row__link {
  color: inherit;
  text-decoration: underline;
  text-decoration-color: transparent;
  text-underline-offset: 3px;
  transition: color 0.2s ease, text-decoration-color 0.2s ease;

  &:hover,
  &:focus-visible {
    color: var(--split-current);
    text-decoration-color: currentColor;
  }

  &:focus-visible {
    border-radius: 3px;
    outline: 2px solid var(--split-current);
    outline-offset: 2px;
  }
}

.split-row__self {
  flex-shrink: 0;
  color: var(--split-current);
}

.device-split__state {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  color: #697886;
  font-size: 12px;
}

.device-split__empty {
  margin: 0;
  color: #939ea9;
  font-size: 12px;
}

.device-split__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 720px) {
  .device-split {
    padding: 12px;
  }

  .split-rows,
  .split-rows.is-mig,
  .split-rows.is-shared {
    grid-template-columns: minmax(0, 1fr);
  }

  .split-row {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 12px;
    padding: 8px 12px;
  }

  .split-row__holder {
    flex-basis: 100%;
    flex-wrap: wrap;
    gap: 4px 12px;
    overflow: visible;
    white-space: normal;
  }

  .split-meters {
    grid-template-columns: max-content minmax(0, 1fr);
  }

  .split-meter__value {
    grid-column: 2;
    margin-top: -6px;
    text-align: left;
  }
}
</style>
