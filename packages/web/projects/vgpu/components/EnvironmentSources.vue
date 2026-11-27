<template>
  <div ref="root" class="environment-sources">
    <div
      role="tablist"
      :aria-label="t('about.sources.vendor')"
      class="source-tabs"
      @keydown="onTabKeydown"
    >
      <button
        v-for="tab in tabs"
        :key="tab"
        :id="tabId(tab)"
        type="button"
        role="tab"
        :aria-selected="selected === tab"
        :aria-controls="panelId"
        :tabindex="selected === tab ? 0 : -1"
        class="source-tab"
        :class="{ 'is-active': selected === tab }"
        @click="selected = tab"
      >
        <span class="vendor-tag">
          <LayersIcon v-if="tab === 'cluster'" aria-hidden="true" />
          <VendorMark v-else :vendor="tab" :size="15" />
          {{ tab === 'cluster' ? t('catalog.cluster') : vendorName(tab) }}
        </span>
      </button>
    </div>
    <div :id="panelId" role="tabpanel" :aria-labelledby="tabId(selected)">
      <div class="component-columns" :class="{ 'vendor-details': selected !== 'cluster' }">
        <section v-for="group in groups" :key="group.key" class="component-column">
          <header class="column-heading">
            <h4>{{ t('about.sources.' + group.key) }}</h4>
            <span v-if="selected === 'cluster' && group.rows.some(row => row.counts)">{{ t('catalog.counts.' + group.key) }}</span>
          </header>
          <template v-if="selected === 'cluster'">
            <div v-if="group.rows.length" class="tile-row">
              <component
                :is="tabs.includes(row.vendor) ? 'button' : 'div'"
                v-for="row in group.rows"
                :key="row.id"
                :type="tabs.includes(row.vendor) ? 'button' : undefined"
                class="component-tile"
                :class="row.tone"
                :title="[row.name, row.version, row.summary].filter(Boolean).join(' · ')"
                :aria-label="[row.name, row.version, row.summary].filter(Boolean).join(' · ')"
                @click="openVendor(row.vendor)"
              >
                <span class="tile-brand">
                  <VendorMark :vendor="row.vendor" :size="20"><LayersIcon aria-hidden="true" /></VendorMark>
                  <ErrorCircleIcon v-if="row.tone === 'is-warning'" class="tile-alert" aria-hidden="true" />
                </span>
                <span v-if="row.counts" class="tile-ratio">{{ row.counts.good }}<small> / {{ row.counts.total }}</small></span>
                <span v-else class="tile-ratio">—</span>
                <span class="tile-version">{{ shortVersion(row.version) }}</span>
              </component>
            </div>
            <p v-else class="source-empty">{{ t('about.sources.' + group.emptyMessage) }}</p>
          </template>
          <template v-else>
            <ul v-if="details[group.key].length" class="detail-list">
              <li v-for="entry in details[group.key]" :key="entry.id">
                <div class="entry-title">
                  <span class="entry-name">{{ entry.name }}</span>
                  <span v-if="entry.archived" class="entry-badge">{{ t('catalog.archived') }}</span>
                  <VersionTag v-if="entry.version" :version="entry.version" />
                </div>
                <span class="entry-state" :class="entry.tone">{{ entry.status }}</span>
                <div v-if="entry.repository || entry.docs" class="entry-links">
                  <a v-if="entry.repository" class="entry-link" :href="entry.repository" target="_blank" rel="noopener noreferrer">
                    {{ repositoryName(entry.repository) }}<JumpIcon aria-hidden="true" />
                  </a>
                  <a v-if="entry.docs" class="entry-link" :href="entry.docs" target="_blank" rel="noopener noreferrer">
                    {{ t('catalog.guide') }}<JumpIcon aria-hidden="true" />
                  </a>
                </div>
              </li>
            </ul>
            <p v-else class="source-empty">{{ t('catalog.noEntry') }}</p>
          </template>
        </section>
      </div>
      <p v-if="selectedCatalog?.componentsOnly" class="coverage-note">
        <span>{{ t('catalog.componentsOnly') }}</span>
        <a class="entry-link" href="https://github.com/Project-HAMi/HAMi-WebUI/blob/main/CONTRIBUTING.md" target="_blank" rel="noopener noreferrer">
          {{ t('catalog.contributeMonitoring') }}<JumpIcon aria-hidden="true" />
        </a>
      </p>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, ref, useId } from 'vue';
import { useI18n } from 'vue-i18n';
import { ErrorCircleIcon, JumpIcon, LayersIcon } from 'tdesign-icons-vue-next';
import VersionTag from './VersionTag.vue';
import { shortVersion } from './about-display.mjs';
import VendorMark from './VendorMark.vue';
import { componentCatalog } from './about-catalog.mjs';

const props = defineProps({ groups: { type: Array, required: true }, checkedKeys: Array });
const selected = defineModel({ type: String, required: true });
const { t, te } = useI18n();
const idPrefix = useId();
const panelId = `${idPrefix}-panel`;
const tabId = (tab) => `${idPrefix}-tab-${tab}`;
const tabs = computed(() => [...new Set(['cluster', ...componentCatalog.map(item => item.vendor),
  ...props.groups.flatMap(group => group.rows.map(row => row.vendor).filter(Boolean))])]);
const vendorName = name => te('catalog.vendors.' + name) ? t('catalog.vendors.' + name) : name;
const root = ref(null);
async function focusActiveTab() {
  await nextTick();
  const activeTab = root.value?.querySelector('[role="tab"][aria-selected="true"]');
  activeTab?.focus();
  activeTab?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}
async function openVendor(vendor) {
  if (!tabs.value.includes(vendor)) return;
  selected.value = vendor;
  await focusActiveTab();
}
async function onTabKeydown(event) {
  const list = tabs.value;
  const current = list.indexOf(selected.value);
  if (current < 0 || list.length === 0) return;
  let next;
  if (event.key === 'ArrowRight') next = (current + 1) % list.length;
  else if (event.key === 'ArrowLeft') next = (current - 1 + list.length) % list.length;
  else if (event.key === 'Home') next = 0;
  else if (event.key === 'End') next = list.length - 1;
  else return;
  event.preventDefault();
  selected.value = list[next];
  await focusActiveTab();
}

const selectedCatalog = computed(() => componentCatalog.find(item => item.vendor === selected.value));
const details = computed(() => {
  const catalog = selectedCatalog.value;
  return Object.fromEntries(props.groups.map(group => {
    const supported = catalog?.[group.key] ?? [];
    const observed = group.rows.filter(row => row.vendor === selected.value);
    const rows = observed.map(row => ({
      ...supported.find(entry => entry.key === row.catalogKey),
      ...row,
      status: row.summary || t('catalog.installed'),
    }));
    for (const entry of supported) {
      if (observed.some(row => row.catalogKey === entry.key)) continue;
      rows.push({ ...entry, id: entry.key, status: t(props.checkedKeys?.includes(entry.key) ? 'catalog.notFound' : 'catalog.unchecked') });
    }
    return [group.key, rows];
  }));
});
function repositoryName(url) {
  return new URL(url).pathname.split('/').slice(1, 3).join('/');
}
</script>

<style scoped>
.environment-sources { min-width: 0; background: white; border: 1px solid #dce4ed; border-radius: 8px; overflow: hidden; }
.source-tabs {
  display: flex; align-items: center; gap: 2px; overflow-x: auto; scrollbar-width: none;
  padding: 5px 9px; border-bottom: 1px solid #e2e8f0;
}
.source-tabs::-webkit-scrollbar { display: none; }
.source-tab {
  flex: 0 0 auto; border: 0; padding: 0; background: transparent; cursor: pointer; font: inherit;
  border-radius: 5px;
}
.source-tab:focus-visible { outline: 2px solid var(--td-brand-color, #2563eb); outline-offset: -2px; }
.vendor-tag { display: inline-flex; align-items: center; gap: 5px; padding: 5px 8px; border-radius: 5px; color: #526477; font-size: 12px; line-height: 18px; }
.vendor-tag svg { width: 15px; height: 15px; }
.source-tab:hover .vendor-tag { background: #f2f5f8; }
.source-tab.is-active .vendor-tag { background: #e9eff6; color: #183455; font-weight: 600; }
.coverage-note { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 4px; margin: 0; padding: 8px 14px; border-top: 1px solid #e2e8f0; color: #526477; font-size: 12px; }
.component-columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
.component-column { min-width: 0; padding: 14px; }
.component-column + .component-column { border-left: 1px solid #e2e8f0; }
.column-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 0 6px; margin-bottom: 10px; }
.column-heading h4 { margin: 0; color: #34485c; font-size: 13px; line-height: 20px; font-weight: 600; }
.column-heading > span { color: #63768a; font-size: 11px; }
.tile-row { display: grid; grid-auto-flow: column; grid-auto-columns: max(64px, calc((100% - 18px) / 4)); justify-content: start; gap: 6px; overflow-x: auto; scrollbar-width: thin; padding-bottom: 2px; }
.component-tile {
  min-width: 0; box-sizing: border-box; display: grid; align-content: start; text-align: start;
  grid-template-rows: 22px 24px 13px; padding: 8px; border: 1px solid transparent; border-radius: 7px;
  background: #f3f6f9; color: #26394d; font: inherit;
}
.component-tile.is-warning { background: #fff5e6; color: #82500f; }
button.component-tile { cursor: pointer; }
button.component-tile:hover { border-color: #bbc8d5; }
button.component-tile.is-warning:hover { border-color: #b77922; }
button.component-tile:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: -2px; }
.tile-brand { display: flex; align-items: center; justify-content: space-between; }
.tile-brand > svg { width: 20px; height: 20px; }
.tile-brand > .tile-alert { width: 13px; height: 13px; color: #a5600a; }
.tile-ratio { align-self: end; font-size: 18px; line-height: 22px; font-weight: 600; font-variant-numeric: tabular-nums; white-space: nowrap; }
.tile-ratio small { font-size: 12px; font-weight: 450; color: #5c7084; }
.is-warning .tile-ratio small { color: inherit; }
.tile-version { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10px; line-height: 13px; color: #607389; }
.source-empty { margin: 0; font-size: 12px; line-height: 18px; color: #63768a; }
.detail-list { padding: 0; margin: 0; list-style: none; }
.detail-list li { display: grid; justify-items: start; gap: 4px; }
.detail-list li + li { border-top: 1px solid #e8edf2; margin-top: 12px; padding-top: 12px; }
.entry-title { display: flex; align-items: center; flex-wrap: wrap; gap: 3px 7px; }
.entry-name { font-size: 14px; line-height: 20px; font-weight: 600; color: #26394d; overflow-wrap: anywhere; }
.entry-badge { max-width: 100%; overflow-wrap: anywhere; border-radius: 4px; padding: 0 5px; background: #f1f4f7; color: #596e81; font-size: 10px; line-height: 16px; font-variant-numeric: tabular-nums; }
.entry-state { display: inline-flex; align-items: center; gap: 6px; color: #596c7e; font-size: 12px; line-height: 18px; }
.entry-state::before { content: ''; box-sizing: border-box; width: 6px; height: 6px; flex: none; border-radius: 50%; border: 1px solid #9cabb9; }
.entry-state.is-ready::before { background: #40936b; border-color: #40936b; }
.entry-state.is-warning { color: #82500f; }
.entry-state.is-warning::before { background: #b77922; border-color: #b77922; }
.entry-links { display: flex; flex-wrap: wrap; align-items: baseline; gap: 3px 14px; max-width: 100%; }
.entry-link { max-width: 100%; overflow-wrap: anywhere; color: #526b82; font-size: 12px; line-height: 18px; text-decoration: none; }
.entry-link svg { display: inline-block; width: 1em; height: 1em; margin-inline-start: 3px; vertical-align: -0.2em; }
.entry-link:hover, .entry-link:focus-visible { color: var(--td-brand-color); text-decoration: underline; }
@media(max-width:480px) {
  .component-column { padding: 10px 8px; }
  .vendor-details { grid-template-columns: 1fr; }
  .vendor-details .component-column + .component-column { border-left: 0; border-top: 1px solid #e2e8f0; }
  .column-heading { align-items: start; flex-direction: column; }
}
</style>
