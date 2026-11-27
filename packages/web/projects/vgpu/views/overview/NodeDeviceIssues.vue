<template>
  <div class="node-issues">
    <p v-if="!hasRows" class="issue-state" role="status">
      {{ status === 'loading' ? t('loading') : status === 'error' ? t('unavailable') : status === 'partial' ? t('partial') : t('empty') }}
    </p>
    <template v-else>
      <div ref="scrollArea" class="issue-scroll" tabindex="0" role="region" :aria-label="t('incidents')">
        <table class="issue-list">
          <thead><tr><th scope="col" class="issue-name">{{ t('node') }}</th><th scope="col">{{ t('issueStatus') }}</th></tr></thead>
          <tbody v-for="(row, index) in pageRows" :key="row.node">
            <tr class="issue-group__heading">
              <th scope="row" class="issue-name">
                <TextPlus v-if="row.to" class="issue-node" :text="row.node" :to="row.to" tooltip="overflow" :tooltip-append-to="tooltipContainer" />
                <span v-else>{{ row.node }}</span>
              </th>
              <td>
                <div class="issue-status-cell">
                  <div class="issue-statuses">
                    <span v-if="row.notReady" class="issue-status">
                      <svg-icon icon="status-unschedulable" aria-hidden="true" />{{ t('nodeNotReady') }}
                    </span>
                    <span v-if="row.devices.length" class="issue-status">
                      <svg-icon icon="status-unschedulable" aria-hidden="true" />{{ deviceStatusLabel(row) }}
                    </span>
                  </div>
                  <button v-if="hasDetails(row)" type="button" class="issue-disclosure"
                    :aria-expanded="expanded === row.node" :aria-controls="detailsId(index)"
                    :title="toggleLabel(row)" :aria-label="toggleLabel(row)" @click="toggle(row)">
                    <ChevronDownIcon aria-hidden="true" />
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="hasDetails(row)" v-show="expanded === row.node" :id="detailsId(index)" class="issue-detail-row">
              <td colspan="2">
                <div v-if="expanded === row.node" class="issue-details">
                  <div v-if="row.devices.some(hasDeviceDetail)" class="issue-detail-field">
                    <span class="issue-field-label">{{ t('affectedGroup') }}</span>
                    <div class="issue-field-value">
                      <div v-for="group in visibleDeviceGroups(row.devices, deviceLimit)" :key="group.model" class="issue-model-group">
                        <p v-if="group.model || row.devices.some(device => device.model)" class="issue-model">{{ group.model || t('modelMissing') }}</p>
                        <ul v-if="group.devices.length" class="issue-device-ids" :aria-label="t('deviceIds')">
                          <li v-for="device in group.devices" :key="device.key">
                            <RouterLink v-if="device.detailTo" class="issue-device-id vgpu-resource-link" :to="device.detailTo">
                              <span>{{ device.deviceId }}</span>
                            </RouterLink>
                            <span v-else class="issue-device-id">{{ device.deviceId }}</span>
                          </li>
                        </ul>
                        <p v-if="group.reportedWithoutId && row.affectedDevices !== group.reportedWithoutId" class="issue-quantity">{{ t('noDeviceId', group.reportedWithoutId) }}</p>
                      </div>
                      <div v-if="row.devices.length > initialDeviceLimit" class="issue-device-actions">
                        <button v-if="deviceLimit < row.devices.length" type="button" :aria-controls="detailsId(index)" @click="showMore(row)">
                          {{ t('showMore', Math.min(deviceStep, row.devices.length - deviceLimit)) }} <ChevronDownIcon aria-hidden="true" />
                        </button>
                        <button v-if="deviceLimit > initialDeviceLimit" type="button" :aria-controls="detailsId(index)" @click="collapse($event)">
                          {{ t('collapse') }} <ChevronUpIcon aria-hidden="true" />
                        </button>
                      </div>
                    </div>
                  </div>
                  <div v-if="inventory(row)" class="issue-detail-field"><span class="issue-field-label">{{ t('nodeDevices') }}</span><span class="issue-field-value">{{ inventory(row) }}</span></div>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <nav v-if="pages > 1" class="issue-pager" :aria-label="t('nodePages')">
        <span aria-live="polite">{{ t('range', { first: (currentPage - 1) * pageSize + 1, last: Math.min(currentPage * pageSize, rows.length), total: rows.length }) }}</span>
        <button type="button" :disabled="currentPage === 1" :aria-label="t('previous')" @click="turnPage(-1, $event)"><ChevronLeftIcon aria-hidden="true" /></button>
        <button type="button" :disabled="currentPage === pages" :aria-label="t('next')" @click="turnPage(1, $event)"><ChevronRightIcon aria-hidden="true" /></button>
      </nav>
    </template>
  </div>
</template>

<script setup>
import { computed, nextTick, ref, useId, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import TextPlus from '@/components/TextPlus.vue';
import { ChevronDownIcon, ChevronLeftIcon, ChevronRightIcon, ChevronUpIcon } from 'tdesign-icons-vue-next';
import { isCount, visibleDeviceGroups } from './node-device-issues.mjs';

const props = defineProps({
  summary: { type: Object, required: true },
  status: { type: String, default: 'loading' },
});
const { t: translate } = useI18n();
const t = (key, ...args) => translate(`dashboard.issueList.${key}`, ...args);
const pageSize = 5;
const initialDeviceLimit = 3;
const deviceStep = 10;
const rows = computed(() => props.summary.incidents);
const hasRows = computed(() => rows.value.length > 0);
const page = ref(1);
const pages = computed(() => Math.max(1, Math.ceil(rows.value.length / pageSize)));
const currentPage = computed(() => Math.min(page.value, pages.value));
const pageRows = computed(() => rows.value.slice((currentPage.value - 1) * pageSize, currentPage.value * pageSize));
const scrollArea = ref(null);
const tooltipContainer = computed(() => scrollArea.value?.closest('.overview-issues-popup'));
const idPrefix = useId();
const detailsId = index => idPrefix + '-devices-' + index;

function inventory(row) {
  const item = row.notReady;
  if (!item) return '';
  if (!isCount(item.deviceCount)) return item.model || '';
  return item.model ? t('inventoryWithModel', { count: item.deviceCount, model: item.model }, item.deviceCount)
    : t('inventoryCount', item.deviceCount);
}

const expanded = ref(null);
const deviceLimit = ref(initialDeviceLimit);
function hasDeviceDetail(device) {
  return Boolean(device.deviceId || device.model);
}
function deviceStatusLabel(row) {
  const known = row.devices.every(device => device.deviceId || isCount(device.deviceCount));
  return known && row.affectedDevices > 0 ? t('affected', row.affectedDevices) : t('deviceIssue');
}
function hasDetails(row) {
  return row.devices.some(hasDeviceDetail) || Boolean(inventory(row));
}
function toggleLabel(row) {
  return t(expanded.value === row.node ? 'collapseNode' : 'expandNode', { node: row.node });
}
function toggle(row) {
  expanded.value = expanded.value === row.node ? null : row.node;
  deviceLimit.value = initialDeviceLimit;
}

async function showMore(row) {
  // Keep focus on the control as the final increment changes it to “show fewer”.
  const focused = document.activeElement;
  const controls = focused?.closest('.issue-device-actions');
  deviceLimit.value = Math.min(deviceLimit.value + deviceStep, row.devices.length);
  await nextTick();
  if (controls && !focused.isConnected) controls.querySelector('button')?.focus({ preventScroll: true });
}
async function collapse(event) {
  const controls = event.currentTarget.parentElement;
  const hadFocus = document.activeElement === event.currentTarget;
  deviceLimit.value = initialDeviceLimit;
  await nextTick();
  if (hadFocus) controls.querySelector('button')?.focus({ preventScroll: true });
}
async function turnPage(delta, event) {
  const button = event.currentTarget;
  const hadFocus = document.activeElement === button;
  page.value = currentPage.value + delta;
  await nextTick();
  if (hadFocus && button.disabled) button.parentElement.querySelector('button:enabled')?.focus({ preventScroll: true });
}
watch(pageRows, async () => {
  const area = scrollArea.value;
  const focused = document.activeElement;
  if (!area?.contains(focused)) return;
  const group = focused.closest('tbody');
  if (!group) return;
  const groups = group.parentElement.tBodies;
  const index = Array.from(groups).indexOf(group);
  await nextTick();
  if (!area.isConnected || focused.isConnected || document.activeElement !== document.body) return;
  const nextGroup = group.isConnected ? group : groups[Math.min(index, groups.length - 1)];
  (nextGroup?.querySelector('a, button') || area).focus({ preventScroll: true });
});
watch(pages, n => { if (page.value > n) page.value = n; });
watch(currentPage, () => {
  expanded.value = null;
  deviceLimit.value = initialDeviceLimit;
  if (scrollArea.value) scrollArea.value.scrollTop = 0;
}, { flush: 'post' });
</script>

<style scoped>
.node-issues { display: flex; flex-direction: column; min-height: 0; color: #324558; font-size: 12px; line-height: 16px; }
.node-issues button { font: inherit; cursor: pointer; }
.issue-pager button:hover:enabled { background: #f1f4f7; }
.issue-scroll { min-height: 0; overflow: auto; overscroll-behavior: contain; scrollbar-width: thin; scrollbar-color: var(--app-scrollbar-thumb, #cbd5e1) transparent; scroll-padding-block: 8px; }
.issue-list { width: calc(100% - 2 * var(--issue-inline-inset)); margin: 0 var(--issue-inline-inset); border-collapse: separate; border-spacing: 0; table-layout: fixed; text-align: left; }
.issue-list thead th { position: sticky; top: 0; z-index: 1; padding: 4px 0; background: white; border-bottom: 1px solid #e3e9ef; color: #65768a; font-size: 11px; font-weight: 500; text-align: left; }
.issue-list .issue-name { overflow-wrap: anywhere; width: max(37%, min(130px, 48%)); padding-inline-end: 10px; box-sizing: border-box; }
.issue-group__heading > :is(th, td) { height: 40px; padding: 4px 0; vertical-align: middle; box-sizing: border-box; text-align: left; }
.issue-list tbody + tbody .issue-group__heading > * { border-top: 1px solid #edf1f5; }
.issue-status-cell { display: grid; grid-template-columns: minmax(0, 1fr) 28px; column-gap: 4px; align-items: center; }
.issue-disclosure { display: grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 4px; color: #617488; background: transparent; }
.issue-disclosure:hover, .issue-disclosure[aria-expanded='true'] { color: #2b4055; background: #f1f4f7; }
.issue-disclosure .t-icon { font-size: 15px; }
.issue-disclosure[aria-expanded='true'] .t-icon { transform: rotate(180deg); }
.issue-node { font-size: 13px; vertical-align: middle; }
.issue-statuses { display: flex; flex-wrap: wrap; gap: 4px 10px; }
.issue-status { display: inline-flex; align-items: center; gap: 6px; color: #526476; }
.issue-status :is(.svg-icon, .t-icon) { flex: none; font-size: 16px; }
.issue-detail-row > td { padding: 0 0 8px; }
.issue-list tbody:last-child .issue-detail-row > td { padding-bottom: 0; }
.issue-details { padding: 8px 12px; border-radius: 4px; background: #f6f8fa; }
.issue-details > :first-child, .issue-model-group > :first-child { margin-top: 0; }
.issue-detail-field { display: grid; grid-template-columns: minmax(58px, max-content) minmax(0, 1fr); align-items: baseline; column-gap: 10px; }
.issue-detail-field + .issue-detail-field { margin-top: 8px; }
.issue-field-label { color: #67788a; }
.issue-field-value { min-width: 0; overflow-wrap: anywhere; }
.issue-details:empty, .issue-model-group:empty { display: none; }
.issue-model-group + .issue-model-group { margin-top: 8px; }
.issue-model { margin: 4px 0 0; color: #506478; overflow-wrap: anywhere; }
.issue-device-ids { display: grid; gap: 4px; margin: 4px 0 0; padding: 0; list-style: none; }
.issue-quantity { margin: 4px 0 0; color: #607386; }
.node-issues .vgpu-resource-link { display: inline-flex; max-width: 100%; align-items: last baseline; }
.node-issues .vgpu-resource-link > span { min-width: 0; overflow-wrap: anywhere; text-decoration: underline; text-decoration-color: var(--resource-link-underline); text-underline-offset: 3px; }
.issue-device-id { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 11px; }
.issue-device-actions { display: flex; gap: 12px; padding-top: 4px; }
.issue-device-actions button { display: inline-flex; align-items: center; gap: 3px; border: 0; border-radius: 3px; background: transparent; padding: 4px 0; color: #405e7a; }
.issue-device-actions button:hover { color: var(--td-brand-color, #2563eb); }
.issue-pager { display: flex; align-items: center; justify-content: flex-end; gap: 4px; flex: none; padding: 8px var(--issue-inline-inset) 0; border-top: 1px solid #e4ebf1; color: #647688; }
.issue-pager span { margin-inline-end: auto; }
.issue-pager button { display: grid; place-items: center; width: 28px; height: 28px; border: 1px solid #dce4eb; background: #fff; color: #475b6e; border-radius: 4px; }
.issue-pager button:disabled { cursor: default; color: #a3afbb; background: #f8fafb; }
.issue-state { padding: 0 var(--issue-inline-inset); margin: 0; color: #627588; }
.node-issues :is(button, .issue-scroll):focus-visible { outline: 2px solid var(--td-brand-color, #2563eb); outline-offset: -2px; }
</style>
