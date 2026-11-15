<template>
  <Block class="usage-block" :title="$t('dashboard.deviceUsageDistribution')">
    <template #title-suffix>
      <MetricHelp
        :help-label="$t('dashboard.deviceUsageHelpLabel')"
        :description="$t('dashboard.deviceUsageHelp', { limit: individualDeviceLimit })"
        multiline
      />
    </template>
    <template #extra>
      <nav v-if="status === 'ready' && groups.length > modelPageSize" class="model-pagination" :aria-label="t('modelPagination')">
        <span class="model-range" aria-live="polite" aria-atomic="true">
          <span class="range-full">{{ modelRange }}</span>
          <span class="range-short" aria-hidden="true">{{ modelPage.current }} / {{ Math.ceil(groups.length / modelPageSize) }}</span>
        </span>
        <button type="button" class="pager-action" :disabled="modelPage.current === 1"
          :aria-label="t('previous')" :title="t('previous')" @click="changeModelPage(-1, $event)">
          <ChevronLeftIcon aria-hidden="true" />
        </button>
        <button type="button" class="pager-action" :disabled="modelPage.current * modelPageSize >= groups.length"
          :aria-label="t('next')" :title="t('next')" @click="changeModelPage(1, $event)">
          <ChevronRightIcon aria-hidden="true" />
        </button>
      </nav>
    </template>
    <div ref="distributionRoot" class="gpu-distribution" tabindex="-1" role="region" :aria-label="$t('dashboard.deviceUsageDistribution')" :class="{ 'has-selection': selection }" @keydown.esc="dismissSelection">
      <div v-if="status !== 'ready' || !groups.length" class="distribution-state" role="status">
        {{ status === 'loading' ? t('loading') : status !== 'ready' ? t('unavailable') : t('empty') }}
      </div>
      <template v-else>
        <div class="distribution-header distribution-grid" aria-hidden="true">
          <span>{{ t('model') }}</span>
          <div v-for="metric in metrics" :key="metric" class="metric-heading">
            <span>{{ t(metric) }}</span>
            <div class="distribution-axis"><span>0</span><span>50</span><span>100%</span></div>
          </div>
        </div>

        <div v-for="group in visibleGroups" :key="group.model" class="distribution-group">
          <div class="distribution-grid distribution-row">
            <div class="model-label">
              <span class="model-identity"><VendorMark v-if="group.model" :vendor="group.vendor" /><span>{{ group.model || t('unknown') }}</span></span>
              <span class="gpu-count">{{ t('gpus', { n: group.count }, group.count) }}</span>
            </div>
            <div v-for="metric in metrics" :key="metric" class="metric-cell">
              <div class="mobile-heading" aria-hidden="true">
                {{ t(metric) }}
                <div class="distribution-axis"><span>0</span><span>50</span><span>100%</span></div>
              </div>
              <template v-if="group[metric].bins || group[metric].dots.length">
                <div v-if="group[metric].bins" class="histogram">
                  <template v-for="(bin, index) in group[metric].bins" :key="index">
                    <button v-if="bin.length" type="button" class="bin"
                      :aria-label="bucketLabel(group, metric, index)" :title="isSelected(group.model, metric, index) ? t('collapseAgain') : bucketLabel(group, metric, index)"
                      :aria-expanded="isSelected(group.model, metric, index)"
                      :aria-controls="isSelected(group.model, metric, index) ? detailsId : undefined"
                      @click="openList($event, group.model, metric, index)">
                      <span class="bar-fill" :style="{ height: (bin.length / maxBinCount * 100) + '%' }"></span>
                    </button>
                    <span v-else class="empty-bin"></span>
                  </template>
                </div>
                <div v-else class="dot-chart" :style="{ '--lanes': group[metric].lanes }">
                  <button v-for="dot in group[metric].dots" :key="dot.value" class="dot" type="button"
                    :class="{ 'label-before': dot.value > 50 }"
                    :style="{ left: dot.value + '%', '--lane': dot.lane }"
                    :title="isSelected(group.model, metric, 'reading', dot.value) ? t('collapseAgain') : dotLabel(group, metric, dot)"
                    :aria-label="dotLabel(group, metric, dot)" :aria-expanded="isSelected(group.model, metric, 'reading', dot.value)"
                    :aria-controls="isSelected(group.model, metric, 'reading', dot.value) ? detailsId : undefined"
                    @click="openList($event, group.model, metric, 'reading', dot.value)">
                    <span v-if="dot.cards.length > 1" class="dot-value" aria-hidden="true">{{ groupCount(dot.cards) }}</span>
                  </button>
                </div>
              </template>
              <div v-if="group[metric].missing.length" class="cell-notes">
                <button type="button" class="missing-count"
                  :title="isSelected(group.model, metric, 'missing') ? t('collapseAgain') : missingLabel(group, metric)" :aria-label="missingLabel(group, metric)"
                  :aria-expanded="isSelected(group.model, metric, 'missing')"
                  :aria-controls="isSelected(group.model, metric, 'missing') ? detailsId : undefined"
                  @click="openList($event, group.model, metric, 'missing')">
                  {{ missingCount(group[metric].missing.length) }}
                </button>
              </div>
            </div>
          </div>

          <section v-if="selection?.model === group.model && selectedDevices.length" class="device-list"
            :id="detailsId" :aria-label="detailTitle">
            <div class="device-list-heading">
              <div class="device-list-summary">
                <span class="detail-metric">{{ t(selection.metric) }}</span>
                <div class="detail-reading">
                  <h4 tabindex="-1" :ref="el => { if (el) detailHeading = el }" :aria-label="detailTitle">{{ detailValue }}</h4>
                  <span class="detail-count">{{ groupCount(selectedDevices) }}</span>
                </div>
              </div>
              <button type="button" class="detail-action detail-collapse" @click="closeList">
                <ChevronUpIcon aria-hidden="true" />{{ t('close') }}
              </button>
            </div>
            <div class="details-scroll" ref="detailScroll" tabindex="0" role="region" :aria-label="t('devices')">
              <table>
                <thead><tr>
                  <th scope="col" class="detail-node-col">{{ t('node') }}</th>
                  <th scope="col" class="detail-device-label">{{ t('device') }}</th>
                  <th v-for="metric in metrics" :key="metric" scope="col" class="detail-rate" :class="{ 'selected-rate': metric === selection.metric }">{{ t(metric) }}</th>
                </tr></thead>
                <tbody v-for="nodeGroup in pagedNodeGroups" :key="nodeGroup.key">
                  <tr class="mobile-node-row">
                    <th scope="rowgroup" colspan="3">
                      <RouterLink v-if="nodeGroup.to" class="vgpu-resource-link" :to="nodeGroup.to"><span>{{ nodeGroup.name }}</span></RouterLink>
                      <span v-else>{{ nodeGroup.name || '—' }}</span>
                    </th>
                  </tr>
                  <tr v-for="(device, index) in nodeGroup.devices" :key="device.key">
                    <th v-if="index === 0" scope="rowgroup" class="detail-node-col" :rowspan="nodeGroup.devices.length">
                      <RouterLink v-if="nodeGroup.to" class="vgpu-resource-link" :to="nodeGroup.to"><span>{{ nodeGroup.name }}</span></RouterLink>
                      <span v-else>{{ nodeGroup.name || '—' }}</span>
                    </th>
                    <td class="detail-identity">
                      <RouterLink v-if="device.detailTo" class="device-id vgpu-resource-link" :to="device.detailTo">
                        <span>{{ device.id }}</span>
                      </RouterLink>
                      <span v-else class="device-id">{{ device.id }}</span>
                    </td>
                    <td v-for="metric in metrics" :key="metric" class="detail-rate" :class="{ 'selected-rate': metric === selection.metric, 'detail-rate-missing': !validRate(device[metric]) }">
                      {{ validRate(device[metric]) ? formatRate(device[metric]) : t('noReading') }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <nav v-if="pageCount > 1" class="page-controls" :aria-label="t('devices')">
              <button type="button" class="detail-action" :disabled="activePage === 1" @click="page = activePage - 1">{{ t('previous') }}</button>
              <span aria-live="polite" aria-atomic="true">{{ activePage }} / {{ pageCount }}</span>
              <button type="button" class="detail-action" :disabled="activePage === pageCount" @click="page = activePage + 1">{{ t('next') }}</button>
            </nav>
          </section>
        </div>
      </template>
    </div>
  </Block>
</template>

<script setup>
import VendorMark from '../../../components/VendorMark.vue';
import { gpuVendor } from '../../../metrics/vendor-colors.mjs';
import { computed, nextTick, onMounted, onBeforeUnmount, ref, useId, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import Block from './Block.vue';
import { ChevronLeftIcon, ChevronRightIcon, ChevronUpIcon } from 'tdesign-icons-vue-next';
import { binWidth, makeUsageSeries, validRate } from './device-usage-series.mjs';
import useLocalPagination from '../../../hooks/useLocalPagination';
import { buildNodeDetailLocation } from '../../node/detail-location.mjs';

const emit = defineEmits(['details-visible']);
const props = defineProps({
  devices: { type: Array, default: () => [] },
  status: { type: String, default: 'loading' },
  individualDeviceLimit: { type: Number, default: 8 },
});
const { t: translate, locale } = useI18n();
const t = (key, ...args) => translate(`dashboard.deviceUsage.${key}`, ...args);
const metrics = ['compute', 'memory'];
const modelPageSize = 4;
const pageSize = 20;
const formatRate = (value) => value.toLocaleString(locale.value, { maximumFractionDigits: 2 }) + '%';

const groups = computed(() => {
  const byModel = new Map();
  for (const device of props.devices) {
    const model = device.model || '';
    if (!byModel.has(model)) byModel.set(model, []);
    byModel.get(model).push(device);
  }
  return Array.from(byModel, ([model, devices]) => {
    const vendor = gpuVendor(devices[0].provider);
    return {
      model, count: devices.length,
      vendor: devices.every(device => gpuVendor(device.provider) === vendor) ? vendor : undefined,
      compute: makeUsageSeries(devices, 'compute', props.individualDeviceLimit),
      memory: makeUsageSeries(devices, 'memory', props.individualDeviceLimit),
    };
  }).sort((a, b) => b.count - a.count || a.model.localeCompare(b.model));
});
const maxBinCount = computed(() => groups.value.reduce((max, group) => {
  for (const metric of metrics) {
    for (const bin of group[metric].bins || []) max = Math.max(max, bin.length);
  }
  return max;
}, 1));
const { pagination: modelPage, pagedTableData: visibleGroups, syncTotalAndClamp } =
  useLocalPagination(groups, { pageSize: modelPageSize });
watch(() => groups.value.length, syncTotalAndClamp, { immediate: true });
const modelRange = computed(() => t('modelRange', {
  start: (modelPage.current - 1) * modelPageSize + 1,
  end: Math.min(modelPage.current * modelPageSize, groups.value.length),
  total: groups.value.length,
}));

const bucketRange = (index) => {
  const start = formatRate(index * binWidth);
  const end = formatRate((index + 1) * binWidth);
  if (index === 0) return t('belowRange', { end });
  return t((index + 1) * binWidth === 100 ? 'closedRange' : 'openRange', { start, end });
};
const metricLabel = (group, metric) => (group.model || t('unknown')) + ' · ' + t(metric);
const groupCount = (list) => t('gpus', { n: list.length }, list.length);
const describeFilter = (group, metric, value, list) => t('detailDescription', {
  model: group.model || t('unknown'), metric: t(metric), value, count: groupCount(list),
});
const bucketLabel = (group, metric, index) => describeFilter(group, metric, bucketRange(index), group[metric].bins[index]);
const dotLabel = (group, metric, dot) => [
  metricLabel(group, metric), formatRate(dot.value),
  ...(dot.cards.length === 1 ? [dot.cards[0].id, dot.cards[0].node] : [groupCount(dot.cards)]),
].filter(Boolean).join(' · ');
const missingCount = (n) => t('missing', { n }, n);
const missingLabel = (group, metric) => [metricLabel(group, metric), missingCount(group[metric].missing.length)].join(' · ');

const selection = ref(null);
const detailsId = useId();
const distributionRoot = ref(null);
let chartResizeObserver;

function positionDetailPointer() {
  const panel = document.getElementById(detailsId);
  if (!panel || !opener?.isConnected) return;
  const source = opener.getBoundingClientRect();
  const target = panel.getBoundingClientRect();
  if (target.width > 0) {
    const fraction = (source.left + source.width / 2 - target.left) / target.width;
    panel.style.setProperty('--detail-pointer-x', (100 * fraction) + '%');
  }
}

function handleOutsideClick(event) {
  if (!selection.value) return;
  const path = event.composedPath();
  if (path.includes(document.getElementById(detailsId))) return;
  // Another mark will toggle or replace the selection in its own click handler.
  if (path.includes(distributionRoot.value) && path.some(el => el instanceof Element && el.matches('button[aria-expanded]'))) return;
  selection.value = null;
  opener = null;
}

onMounted(() => {
  document.addEventListener('click', handleOutsideClick, true);
  chartResizeObserver = new ResizeObserver(positionDetailPointer);
  chartResizeObserver.observe(distributionRoot.value);
});
onBeforeUnmount(() => {
  document.removeEventListener('click', handleOutsideClick, true);
  chartResizeObserver?.disconnect();
});
watch(selection, value => {
  emit('details-visible', Boolean(value));
  if (value) positionDetailPointer();
}, { flush: 'post' });
const detailHeading = ref(null);
const detailScroll = ref([]);
const page = ref(1);
let opener;
const selectedGroup = computed(() => visibleGroups.value.find((group) => group.model === selection.value?.model));
const selectedDevices = computed(() => {
  if (!selectedGroup.value) return [];
  const { metric, filter } = selection.value;
  const series = selectedGroup.value[metric];
  if (filter === 'reading') return series.dots.find((dot) => dot.value === selection.value.reading)?.cards || [];
  if (filter === 'missing') return series.missing;
  return series.bins?.[filter] || [];
});
const pageCount = computed(() => Math.ceil(selectedDevices.value.length / pageSize));
const activePage = computed(() => Math.min(page.value, pageCount.value));
const pagedNodeGroups = computed(() => {
  const ordered = [...selectedDevices.value].sort((a, b) =>
    (a.node || '').localeCompare(b.node || '') ||
    (a.nodeUid || '').localeCompare(b.nodeUid || '') ||
    a.id.localeCompare(b.id));
  const start = (activePage.value - 1) * pageSize;
  const byNode = new Map();
  for (const device of ordered.slice(start, start + pageSize)) {
    const key = device.nodeUid ? 'uid:' + device.nodeUid : device.node ? 'name:' + device.node : 'device:' + device.id;
    if (!byNode.has(key)) {
      byNode.set(key, {
        key, name: device.node || device.nodeUid,
        to: buildNodeDetailLocation({ uid: device.nodeUid, nodeName: device.node }),
        devices: [],
      });
    }
    byNode.get(key).devices.push(device);
  }
  return Array.from(byNode.values());
});
const detailValue = computed(() => {
  if (!selectedDevices.value.length) return '';
  const { metric, filter } = selection.value;
  if (filter === 'missing') return t('noReading');
  return Number.isInteger(filter) ? bucketRange(filter) : formatRate(selectedDevices.value[0][metric]);
});
const detailTitle = computed(() => selectedDevices.value.length
  ? describeFilter(selectedGroup.value, selection.value.metric, detailValue.value, selectedDevices.value) : '');
watch(selectedDevices, async (devices) => {
  if (!selection.value || devices.length) return;
  const focused = document.activeElement;
  const restoreFocus = focused === opener || document.getElementById(detailsId)?.contains(focused);
  const metricCell = opener?.closest('.metric-cell');
  selection.value = null;
  opener = null;
  if (!restoreFocus) return;

  await nextTick();
  if (focused.isConnected || document.activeElement !== document.body) return;
  const mark = metricCell?.isConnected && metricCell.querySelector('button[aria-expanded]');
  (mark || distributionRoot.value)?.focus({ preventScroll: true });
});
watch(activePage, () => {
  if (detailScroll.value[0]) detailScroll.value[0].scrollTop = 0;
}, { flush: 'post' });
async function changeModelPage(delta, event) {
  const trigger = event.currentTarget;
  const hadFocus = document.activeElement === trigger;
  selection.value = null;
  opener = null;
  modelPage.current += delta;
  await nextTick();
  if (hadFocus && trigger.disabled) trigger.parentElement?.querySelector('button:enabled')?.focus({ preventScroll: true });
}
function isSelected(model, metric, filter, reading) {
  const current = selection.value;
  return Boolean(current && current.model === model && current.metric === metric && current.filter === filter && current.reading === reading);
}
async function openList(event, model, metric, filter, reading) {
  if (isSelected(model, metric, filter, reading)) return closeList();
  opener = event.currentTarget;
  selection.value = { model, metric, filter, reading };
  page.value = 1;
  if (event.detail === 0) {
    await nextTick();
    detailHeading.value?.focus({ preventScroll: true });
    detailHeading.value?.scrollIntoView({ block: 'nearest' });
  }
}
function dismissSelection(event) {
  if (!selection.value) return;
  event.stopPropagation();
  closeList();
}
function closeList() {
  selection.value = null;
  opener?.focus({ preventScroll: true });
}
</script>

<style scoped>
.gpu-distribution {
  container: usage-distribution / inline-size;
  min-width: 0;
  width: 100%;
  color: #324558;
  font-size: 12px;
  line-height: 1.45;
  --mark: var(--el-color-primary, #3b82f6);
}
.distribution-grid {
  display: grid;
  grid-template-columns: clamp(155px, 24cqi, 310px) repeat(2, minmax(0, 1fr));
  align-items: center;
  column-gap: clamp(12px, 3.5cqi, 24px);
}
.distribution-header { color: #697886; padding-bottom: 6px; }
.distribution-header > span { align-self: start; }
.distribution-axis {
  display: flex;
  justify-content: space-between;
  position: relative;
  margin-top: 6px;
  color: #758492;
  font-size: 10px;
  line-height: 14px;
  font-variant-numeric: tabular-nums;
}
.distribution-axis span:nth-child(2) { position: absolute; left: 50%; transform: translateX(-50%); }
.distribution-group { border-top: 1px solid #edf1f5; }
.distribution-row { min-height: 58px; padding-block: 9px; box-sizing: border-box; }
.model-identity { display: inline-flex; gap: 6px; align-items: center; min-width: 0; }
.model-label { display: grid; gap: 2px; min-width: 0; }
.model-label > :first-child { font-size: 13px; font-weight: 500; overflow-wrap: anywhere; }
.gpu-count { color: #697886; font-variant-numeric: tabular-nums; font-size: 11px; }
.metric-heading, .metric-cell { min-width: 0; padding-inline: 9px; }
.metric-cell { align-self: stretch; display: flex; flex-direction: column; justify-content: flex-start; }
.mobile-heading { display: none; }
.gpu-distribution button { font-family: inherit; }
.gpu-distribution button:focus-visible, .details-scroll:focus-visible { outline: 2px solid var(--mark); outline-offset: 2px; }
.histogram, .dot-chart {
  height: 28px;
  box-sizing: border-box;
  border: 0;
  border-bottom: 1px solid #cbd6df;
  background: linear-gradient(90deg, transparent calc(50% - .5px), #edf1f5 50%, transparent calc(50% + .5px));
}
.histogram { display: grid; grid-template-columns: repeat(10, minmax(0, 1fr)); align-items: end; }
.bin {
  position: relative;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  height: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  cursor: pointer;
}
.bar-fill { width: 84%; min-height: 2px; background: var(--mark); opacity: .78; border-radius: 2px 2px 0 0; }
.bin:hover, .bin[aria-expanded='true'] { background-color: #f2f7ff; }
.bin:hover .bar-fill, .bin:focus-visible .bar-fill, .bin[aria-expanded='true'] .bar-fill { opacity: 1; }
.dot-chart { position: relative; height: max(28px, calc(var(--lanes) * 18px + 1px)); }
.dot { display: grid; place-items: center; position: absolute; bottom: calc(var(--lane) * 18px); width: 18px; height: 18px; transform: translateX(-50%); padding: 0; border: 0; border-radius: 50%; color: #506478; background: transparent; font: inherit; cursor: pointer; }
.dot::after { content: ''; width: 7px; height: 7px; border-radius: 50%; background: var(--mark); }
.dot-value { font-size: 11px; line-height: 16px; font-variant-numeric: tabular-nums; }
.dot-chart .dot-value { position: absolute; left: 100%; white-space: nowrap; }
.dot-chart .label-before .dot-value { left: auto; right: 100%; }
.dot:hover::after, .dot:focus-visible::after, .dot[aria-expanded='true']::after { box-shadow: 0 0 0 2px #dae9ff; }
.cell-notes { display: flex; align-items: baseline; flex-wrap: wrap; gap: 2px 8px; padding-top: 4px; margin-top: auto; color: #697886; font-size: 11px; }
.missing-count, .detail-action { color: #506478; font-size: inherit; border: 0; background: none; padding: 0; cursor: pointer; }
.missing-count { margin-inline-start: auto; color: #697886; text-align: right; }
.missing-count:hover, .missing-count[aria-expanded='true'] { color: var(--mark); }
.has-selection .bin[aria-expanded='false'] .bar-fill,
.has-selection .dot[aria-expanded='false']::after { background: #a2aebc; opacity: 1; }
.has-selection .bin[aria-expanded='false']:hover { background: #f2f4f6; }
.has-selection .dot[aria-expanded='false']:hover::after { box-shadow: 0 0 0 2px #e4e9ef; }
.has-selection .missing-count[aria-expanded='false']:hover { color: #405367; }
.detail-collapse { display: inline-flex; align-items: center; align-self: flex-start; gap: 3px; min-height: 24px; margin-top: -3px; }
.detail-collapse .t-icon { font-size: 14px; }
.model-pagination { display: flex; justify-content: flex-end; align-items: center; gap: 4px; }
.model-range { color: #697886; white-space: nowrap; margin-right: 4px; font-variant-numeric: tabular-nums; }
.pager-action { display: grid; place-items: center; box-sizing: border-box; width: 28px; height: 28px; padding: 0; border: 1px solid #dce3ea; border-radius: 4px; color: #506478; background: #fff; font-size: inherit; cursor: pointer; }
.pager-action:hover:enabled { color: var(--el-color-primary); border-color: var(--el-color-primary); }
.pager-action:focus-visible { outline: 2px solid var(--el-color-primary); outline-offset: 2px; }
.usage-block :deep(.home-block-header) { flex-wrap: wrap; gap: 8px 12px; }
.usage-block { container: usage-card / inline-size; }
.range-short { display: none; }
@container usage-card (max-width: 390px) {
  .range-full { position: absolute; width: 1px; height: 1px; clip-path: inset(50%); overflow: hidden; }
  .range-short { display: inline; }
}
.pager-action:disabled { color: #9ca7b3; background: #f8fafc; cursor: default; }
.distribution-state { min-height: 210px; display: grid; place-items: center; text-align: center; color: #697886; }
.device-list {
  --detail-edge: #cbd6df;
  --pointer-center: clamp(10px, var(--detail-pointer-x, 50%), calc(100% - 10px));
  position: relative;
  margin: 6px 0 12px;
  padding: 12px 14px 8px;
  background-color: #f7f9fc;
  background-image: linear-gradient(to right, transparent calc(var(--pointer-center) - 44px), var(--detail-edge) calc(var(--pointer-center) - 10px), var(--detail-edge) calc(var(--pointer-center) + 10px), transparent calc(var(--pointer-center) + 44px));
  background-size: 100% 1px;
  background-repeat: no-repeat;
  border-radius: 6px;
}
.device-list::before {
  content: '';
  position: absolute;
  top: -4.5px;
  left: var(--pointer-center);
  width: 10px;
  height: 10px;
  box-sizing: border-box;
  transform: translateX(-50%) rotate(45deg);
  background-color: inherit;
  border-top: 1px solid var(--detail-edge);
  border-left: 1px solid var(--detail-edge);
  pointer-events: none;
}
.device-list-heading, .page-controls { display: flex; gap: 12px; align-items: center; justify-content: space-between; }
.device-list-summary { min-width: 0; }
.detail-metric { display: block; margin-bottom: 3px; color: #596c80; font-size: 12px; }
.detail-reading { display: flex; align-items: baseline; flex-wrap: wrap; gap: 3px 12px; }
.device-list-heading h4 {
  margin: 0; min-width: 0; overflow-wrap: anywhere; color: #26374a;
  font-size: 16px; font-weight: 600; line-height: 1.4; font-variant-numeric: tabular-nums;
}
.detail-count { color: #596c80; font-size: 12px; white-space: nowrap; font-variant-numeric: tabular-nums; }
.detail-action { flex-shrink: 0; }
.detail-action:enabled:is(:hover, :focus-visible) {
  color: var(--el-color-primary);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.details-scroll { max-height: 215px; margin-top: 12px; overflow: auto; scrollbar-width: thin; scrollbar-color: var(--app-scrollbar-thumb, #cbd5e1) transparent; }
.details-scroll table { width: 100%; border-collapse: collapse; table-layout: fixed; color: #324558; font-size: 12px; font-variant-numeric: tabular-nums; }
.details-scroll :is(th, td) { padding: 7px 8px; text-align: left; vertical-align: middle; overflow-wrap: anywhere; }
.details-scroll thead { border-bottom: 1px solid #e3e9f0; }
.details-scroll .detail-node-col { padding-left: 0; }
.details-scroll .detail-device-label { width: 43%; }
.details-scroll .detail-node-col { width: 19%; }
.details-scroll .detail-rate { text-align: right; }
.details-scroll .detail-rate-missing { color: #687988; }
.details-scroll thead th { position: sticky; top: 0; z-index: 1; background: #f7f9fc; color: #596c80; font-size: 11px; font-weight: 500; }
.details-scroll .selected-rate { font-weight: 650; color: #24364a; }
.details-scroll tbody th { font-weight: 500; vertical-align: top; }
.details-scroll tbody + tbody { border-top: 1px solid #e3e9f0; }
.details-scroll .vgpu-resource-link { display: inline-flex; align-items: last baseline; max-width: 100%; }
.details-scroll .vgpu-resource-link > span {
  min-width: 0;
  text-decoration: underline;
  text-decoration-color: var(--resource-link-underline);
  text-underline-offset: 3px;
}
.device-id { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 12px; }
.mobile-node-row { display: none; }
.page-controls { justify-content: flex-end; margin-top: 8px; }
.page-controls button:disabled { color: #939ea9; cursor: default; }
@container usage-distribution (max-width: 570px) {
  .device-list { padding: 12px 10px 6px; }
  .details-scroll :is(th, td) { padding-inline: 4px; }
  .details-scroll :is(th, td):last-child { padding-right: 0; }
  .details-scroll .detail-device-label { width: 50%; padding-left: 0; }
  .details-scroll .detail-node-col { display: none; }
  .mobile-node-row { display: table-row; }
  .details-scroll .mobile-node-row th { padding: 6px 0 2px; }
  .details-scroll .detail-identity { padding-left: 0; }
  .distribution-header { display: none; }
  .distribution-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; row-gap: 6px; }
  .model-label { grid-column: 1 / -1; display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 8px; }
  .mobile-heading { display: block; color: #697886; font-size: 11px; }
  .distribution-axis { margin-top: 2px; }
  .distribution-header + .distribution-group { border-top: 0; }
}
@container usage-distribution (max-width: 365px) {
  .distribution-grid { grid-template-columns: minmax(0, 1fr); }
}
</style>
