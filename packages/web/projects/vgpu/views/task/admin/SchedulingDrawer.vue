<template>
  <t-drawer
    :visible="Boolean(identityPod)"
    :header="t('scheduling.title')"
    :footer="false"
    :close-btn="true"
    size="min(680px, 100vw)"
    drawer-class-name="scheduling-drawer"
    :close-on-overlay-click="true"
    :close-on-esc-keydown="true"
    :role="identityPod ? 'dialog' : undefined"
    :aria-label="identityPod ? t('scheduling.title') : undefined"
    :aria-modal="identityPod ? 'true' : undefined"
    :aria-hidden="identityPod ? undefined : 'true'"
    :tabindex="identityPod ? 0 : -1"
    @keydown="onDialogKeydown"
    @close="$emit('close')"
  >
    <template #closeBtn><button type="button" class="scheduling-close" :aria-label="t('scheduling.close')"><CloseIcon aria-hidden="true" /></button></template>
    <section v-if="identityPod" ref="content" class="scheduling-content">
      <header class="sd-header">
        <div class="sd-heading">
          <div class="sd-title-line">
            <h2 class="sd-title">{{ identityPod.name }}</h2>
            <span v-if="!identityError" class="sd-state">
              <svg-icon :icon="tone === 'success' ? 'status-schedulable' : 'status-unmanaged'" aria-hidden="true" />{{ t(`scheduling.stage.${stage}`) }}
            </span>
          </div>
          <p class="sd-subtitle">{{ t('scheduling.namespace') }}: {{ identityPod.namespace }}</p>
        </div>
        <t-button variant="outline" :aria-label="t('scheduling.refresh')" :aria-busy="loading ? 'true' : 'false'" @click="refresh">
          <template #icon><RefreshIcon class="sd-refresh-icon" :class="{ 'is-loading': loading }" /></template>
        </t-button>
      </header>

      <p v-if="identityError" class="sd-notice" role="alert">{{ t(`scheduling.${identityError}`) }}</p>
      <template v-else>
        <p v-if="loadError" class="sd-notice" role="alert">{{ t(response ? 'scheduling.previous' : 'scheduling.loadFailed') }}</p>

        <div class="scheduling-summary sd-conclusion" :data-scheduling-stage="stage">
          <div class="sd-conclusion-main">
            <p class="sd-headline">{{ headline }}</p>
            <ul v-if="reasonItems.length" class="sd-reasons">
              <li v-for="reason in reasonItems" :key="reason.key">
                <span class="sd-reason-icon"><svg-icon :icon="reason.icon" aria-hidden="true" /></span>{{ reason.label }}
              </li>
            </ul>
            <div v-if="stage === 'gated' && (pod.gates || []).length" class="sd-gates">
              <span class="sd-gates-label">{{ t('scheduling.gates') }}</span>
              <code v-for="gate in pod.gates" :key="gate" class="sd-chip">{{ gate }}</code>
            </div>
            <p v-if="pod.preallocated && !pod.nodeName" class="sd-note">{{ t('scheduling.preallocated') }}</p>
            <p v-if="pod.nodeName && allocatedLinks.length" class="sd-note sd-links">
              <RouterLink v-for="link in allocatedLinks" :key="link.container" :to="link.to">
                {{ t('scheduling.openAllocated', { name: link.container }) }}
              </RouterLink>
            </p>
          </div>
          <dl class="sd-facts">
            <div v-if="waitDuration"><dt>{{ t('scheduling.waited') }}</dt><dd>{{ waitDuration }}</dd></div>
            <div v-if="pod.nodeName"><dt>{{ t('scheduling.node') }}</dt><dd>{{ pod.nodeName }}</dd></div>
            <div>
              <dt>{{ t('scheduling.scheduler') }} <MetricHelp :description="t('scheduling.schedulerHelp')" :help-label="t('scheduling.scheduler')" /></dt>
              <dd>{{ pod.schedulerName || '--' }}</dd>
            </div>
          </dl>
        </div>

        <section v-if="checks.length" class="sd-section">
          <h3 class="sd-section-title">{{ t('scheduling.checks') }}</h3>
          <ol class="sd-checks">
            <li v-for="check in checks" :key="check">{{ check }}</li>
          </ol>
        </section>

        <section v-if="requestCards.length" class="sd-section">
          <h3 class="sd-section-title">{{ t('scheduling.requests') }}</h3>
          <div v-for="card in requestCards" :key="`${card.request.containerKind}/${card.request.container}`" class="sd-request">
            <p v-if="requestCards.length > 1 || card.request.containerKind !== 'regular'" class="sd-request-name">
              {{ card.request.container }}<span v-if="card.request.containerKind !== 'regular'" class="sd-request-kind">{{ t(`scheduling.containerKind.${card.request.containerKind}`) }}</span>
            </p>
            <div class="sd-cards">
              <div v-for="tile in card.tiles" :key="`${tile.kind}/${tile.name || ''}`" class="sd-card">
                <span class="sd-card-icon"><svg-icon :icon="REQUEST_ICONS[tile.kind]" aria-hidden="true" /></span>
                <span class="sd-card-info">
                  <span class="sd-card-value" :class="{ 'is-unset': tile.value === null }">{{ tile.value ?? t('scheduling.notSet') }}</span>
                  <span class="sd-card-label">{{ tile.kind === 'raw' ? tile.name : t(`scheduling.resourceKind.${tile.kind}`) }}</span>
                </span>
              </div>
            </div>
          </div>
        </section>

        <section v-if="hasConstraints" class="sd-section">
          <h3 class="sd-section-title">{{ t('scheduling.constraints') }}</h3>
          <dl class="sd-constraints">
            <div v-if="constraintView.nodeSelector.length">
              <dt>{{ t('scheduling.constraintGroup.nodeSelector') }}</dt>
              <dd><code v-for="item in constraintView.nodeSelector" :key="item" class="sd-chip">{{ item }}</code></dd>
            </div>
            <div v-for="item in constraintView.device" :key="item.name">
              <dt>{{ t(`scheduling.deviceConstraint.${item.key}`) }}</dt>
              <dd><code v-for="(value, index) in item.values" :key="index" class="sd-chip" :title="item.name">{{ value }}</code></dd>
            </div>
            <div v-if="constraintView.priorityClassName">
              <dt>{{ t('scheduling.constraintGroup.priorityClassName') }}</dt>
              <dd><code class="sd-chip">{{ constraintView.priorityClassName }}</code></dd>
            </div>
            <div v-if="constraintView.tolerations.length">
              <dt>{{ t('scheduling.constraintGroup.tolerations') }}</dt>
              <dd><code v-for="item in constraintView.tolerations" :key="item" class="sd-chip">{{ item }}</code></dd>
            </div>
            <div v-for="(rule, ruleIndex) in constraintView.nodeAffinity" :key="`node-${ruleIndex}`">
              <dt>{{ t('scheduling.withMode', { name: t('scheduling.constraintGroup.nodeAffinity'), mode: t(`scheduling.affinityMode.${rule.mode}`) }) }}</dt>
              <dd class="sd-terms">
                <div v-for="(term, termIndex) in rule.terms" :key="termIndex" class="sd-term">
                  <span v-if="rule.weights" class="sd-term-note">{{ t('scheduling.weight', { weight: rule.weights[termIndex] }) }}</span>
                  <code v-for="expression in term" :key="expression" class="sd-chip">{{ expression }}</code>
                </div>
                <span v-if="rule.mode === 'required' && rule.terms.length > 1" class="sd-term-note">{{ t('scheduling.anyTerm') }}</span>
              </dd>
            </div>
            <div v-for="(rule, ruleIndex) in constraintView.podAffinity" :key="`pod-${ruleIndex}`">
              <dt>{{ t('scheduling.withMode', { name: t(`scheduling.constraintGroup.${rule.kind}`), mode: t(`scheduling.affinityMode.${rule.mode}`) }) }}</dt>
              <dd class="sd-terms">
                <div v-for="(term, termIndex) in rule.terms" :key="termIndex" class="sd-term">
                  <span v-if="term.weight !== undefined" class="sd-term-note">{{ t('scheduling.weight', { weight: term.weight }) }}</span>
                  <code v-for="expression in term.selector" :key="expression" class="sd-chip">{{ expression }}</code>
                  <span v-if="term.topologyKey" class="sd-term-note">{{ t('scheduling.topologyKey', { key: term.topologyKey }) }}</span>
                  <span v-if="term.namespaces.length" class="sd-term-note">{{ t('scheduling.namespaces', { names: term.namespaces.join(', ') }) }}</span>
                </div>
              </dd>
            </div>
            <div v-if="constraintView.topology.length">
              <dt>{{ t('scheduling.constraintGroup.topology') }}</dt>
              <dd class="sd-terms">
                <div v-for="(item, index) in constraintView.topology" :key="index" class="sd-term">
                  <code class="sd-chip">{{ item.topologyKey }}</code>
                  <span class="sd-term-note">{{ t('scheduling.maxSkew', { skew: item.maxSkew }) }}</span>
                  <span v-if="item.whenUnsatisfiable" class="sd-term-note">{{ item.whenUnsatisfiable }}</span>
                </div>
              </dd>
            </div>
            <div v-if="constraintView.gates.length && stage !== 'gated'">
              <dt>{{ t('scheduling.gates') }}</dt>
              <dd><code v-for="gate in constraintView.gates" :key="gate" class="sd-chip">{{ gate }}</code></dd>
            </div>
            <div v-for="item in constraintView.other" :key="item.name">
              <dt class="sd-dt-code">{{ item.name }}</dt>
              <dd><code class="sd-chip">{{ item.value }}</code></dd>
            </div>
          </dl>
          <details v-if="rawDefinitions.length" class="sd-raw">
            <summary>{{ t('scheduling.rawDefinition') }}</summary>
            <template v-for="item in rawDefinitions" :key="item.name">
              <p class="sd-code-title">{{ item.name }}</p>
              <pre class="sd-code"><span v-for="(token, index) in item.tokens" :key="index" :class="`sd-token-${token.type}`">{{ token.text }}</span></pre>
            </template>
          </details>
        </section>

        <p v-if="loading" class="sd-status-line" role="status">{{ t('scheduling.loading') }}</p>
        <p v-if="response && eventStatus !== 'available'" class="sd-status-line" role="status">{{ t(`scheduling.eventStatus.${eventStatus}`) }}</p>

        <details class="scheduling-records sd-records">
          <summary>
            <span class="sd-section-title">{{ t('scheduling.records') }}</span>
            <span v-if="recordCount" class="sd-records-count">{{ t('scheduling.recordCount', { count: recordCount }, recordCount) }}</span>
          </summary>
          <p v-if="response?.eventsIncomplete" class="sd-notice" role="status">{{ t('scheduling.recordsIncomplete') }}</p>
          <ol v-if="recordCount" class="sd-timeline">
            <li v-if="pod.condition?.message || pod.condition?.reason" class="sd-record is-condition">
              <div class="sd-record-head">
                <strong class="sd-record-title">{{ t('scheduling.conditionRecord') }}</strong>
                <span v-if="pod.condition.reason" class="sd-record-source">{{ pod.condition.reason }}</span>
                <span v-if="pod.condition.transitionAt" class="sd-record-time">
                  <time :datetime="pod.condition.transitionAt" :title="formatDate(pod.condition.transitionAt)">{{ t('scheduling.changedAgo', { time: formatAgo(pod.condition.transitionAt) }) }}</time>
                  <MetricHelp :description="t('scheduling.conditionTimeHelp')" :help-label="t('scheduling.conditionTransition')" />
                </span>
                <t-button class="sd-copy" size="small" variant="text" shape="square" :aria-label="t('scheduling.copyRecord')" :title="t('scheduling.copyRecord')" @click="copyRecord('condition', pod.condition)">
                  <template #icon><CheckIcon v-if="copiedKey === 'condition'" /><CopyIcon v-else /></template>
                </t-button>
              </div>
              <pre v-if="pod.condition.message" class="sd-message">{{ pod.condition.message }}</pre>
              <p v-if="pod.condition.messageTruncated" class="sd-footnote">{{ t('scheduling.messageTruncated') }}</p>
            </li>
            <li v-for="event in visibleEvents" :key="event.uid" class="sd-record" :class="{ 'is-warning': event.type === 'Warning' }" tabindex="-1">
              <span class="sd-sr-only">{{ ['Warning', 'Normal'].includes(event.type) ? t(`scheduling.eventType.${event.type}`) : event.type }}</span>
              <div class="sd-record-head">
                <strong class="sd-record-title">{{ event.reason || event.type }}</strong>
                <span class="sd-record-source">{{ event.source || '--' }}</span>
                <span class="sd-record-time">
                  <template v-if="Number(event.count) > 1">{{ t('scheduling.repetitions', { count: event.count }) }}<span class="sd-time-separator" aria-hidden="true" /></template>
                  <time v-if="event.lastObservedAt" :datetime="event.lastObservedAt" :title="formatDate(event.lastObservedAt)">{{ formatAgo(event.lastObservedAt) }}</time>
                </span>
                <t-button class="sd-copy" size="small" variant="text" shape="square" :aria-label="t('scheduling.copyRecord')" :title="t('scheduling.copyRecord')" @click="copyRecord(event.uid, event)">
                  <template #icon><CheckIcon v-if="copiedKey === event.uid" /><CopyIcon v-else /></template>
                </t-button>
              </div>
              <pre class="sd-message">{{ event.message }}</pre>
              <p v-if="event.messageTruncated" class="sd-footnote">{{ t('scheduling.messageTruncated') }}</p>
            </li>
          </ol>
          <t-button v-if="!showAllEvents && (response?.events || []).length > 20" class="sd-more" variant="text" theme="primary" @click="showMoreEvents">
            {{ t('scheduling.showMoreEvents', { count: response.events.length - 20 }) }}
          </t-button>
          <p class="sd-footnote">{{ eventFootnote }}</p>
          <p v-if="copyStatus" :class="copyStatus === 'copied' ? 'sd-sr-only' : 'sd-footnote'" role="status">{{ t(`scheduling.${copyStatus}`) }}</p>
        </details>
      </template>
    </section>
  </t-drawer>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { RouterLink } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { CheckIcon, CloseIcon, CopyIcon, RefreshIcon } from 'tdesign-icons-vue-next';
import taskApi from '~/vgpu/api/task';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import { deviceWording, sharedVendor } from '~/vgpu/components/device-copy.mjs';
import {
  describeSchedulingConstraints, formatSchedulingAgo, formatSchedulingDuration, getSchedulingChecks, getSchedulingDetailError,
  getSchedulingReasons, getSchedulingRequestTiles, getSchedulingSummary, schedulingIdentity, tokenizeJSON,
} from './scheduling-display.mjs';
import { buildWorkloadDetailLocation } from './workload-identity.mjs';

const props = defineProps({
  identityPod: { type: Object, default: null },
  containerName: { type: String, default: '' },
  focusReturnTarget: { type: Object, default: null },
});
const emit = defineEmits(['close', 'updated']);
const { t: translate, locale } = useI18n();
// Every drawer string names the requested device.
const t = (...args) => deviceWording(translate(...args), requestVendor.value);
const response = ref(null);
const loading = ref(false);
const loadError = ref(false);
const identityError = ref('');
const copyStatus = ref('');
const copiedKey = ref('');
const showAllEvents = ref(false);
const content = ref(null);
let generation = 0;
let controller;
let opener;

const pod = computed(() => response.value?.pod || props.identityPod || {});
const allocatedLinks = computed(() => (pod.value.allocatedContainers || [])
  .map((container) => ({ container, to: buildWorkloadDetailLocation({ podUid: pod.value.uid, name: container }) }))
  .filter((link) => link.to));
const stage = computed(() => ['terminating', 'finished'].includes(pod.value.stage)
  ? pod.value.stage : (pod.value.nodeName ? 'bound' : (pod.value.stage || 'unknown')));
const summary = computed(() => getSchedulingSummary(pod.value, t, props.containerName));
const reasons = computed(() => getSchedulingReasons(pod.value, t));
const checks = computed(() => getSchedulingChecks(pod.value, t, props.containerName));
const now = ref(Date.now());
const waitDuration = computed(() => (['waiting', 'gated'].includes(stage.value) ? formatSchedulingDuration(pod.value.createdAt, now.value, t) : ''));
const formatAgo = (value) => formatSchedulingAgo(value, now.value, t) || formatDate(value);
const hasAllocation = computed(() => {
  const allocated = pod.value.allocatedContainers || [];
  return props.containerName ? allocated.includes(props.containerName) : allocated.length > 0;
});
const tone = computed(() => {
  if (stage.value === 'waiting') return 'warning';
  if (stage.value === 'bound') return hasAllocation.value ? 'success' : 'warning';
  return 'neutral';
});
const REASON_ICONS = Object.freeze({
  memory: 'node-memory-total', hostMemory: 'node-memory-total', cpu: 'node-cpu-total',
  core: 'vgpu-core', coreExhausted: 'vgpu-core',
  slots: 'vgpu-card', devices: 'vgpu-card', model: 'vgpu-card', uuid: 'vgpu-card', health: 'vgpu-card', cordoned: 'vgpu-card',
  exclusive: 'vgpu-card', numa: 'vgpu-card', mig: 'vgpu-card', customFilter: 'vgpu-card', mode: 'vgpu-card',
  quota: 'vgpu-resource', extendedResource: 'vgpu-resource', pvc: 'vgpu-resource', pvcMissing: 'vgpu-resource', volumeAffinity: 'vgpu-resource',
  taint: 'vgpu-node', affinity: 'vgpu-node', nodeUnschedulable: 'vgpu-node', podAffinity: 'vgpu-node', podCapacity: 'vgpu-node', hostPort: 'vgpu-node',
});
const reasonItems = computed(() => {
  if (stage.value !== 'waiting' || !reasons.value.some((reason) => REASON_ICONS[reason.key])) return [];
  return reasons.value.map((reason) => ({ ...reason, icon: REASON_ICONS[reason.key] || 'help-circle' }));
});
const headline = computed(() => {
  const count = reasonItems.value.length;
  return count ? t('scheduling.reasonCount', { count }, count) : summary.value;
});
const REQUEST_ICONS = Object.freeze({
  count: 'vgpu-card', core: 'vgpu-core', memory: 'node-memory-total', memory_percentage: 'node-memory-total', raw: 'vgpu-resource',
});
const requestCards = computed(() => (pod.value.requests || []).map((request) => ({ request, tiles: getSchedulingRequestTiles(request) })));
const requestVendor = computed(() => sharedVendor(requestCards.value.flatMap(({ tiles }) => tiles.filter((tile) => tile.value !== null).map((tile) => tile.vendor))));
const constraintView = computed(() => describeSchedulingConstraints(pod.value.constraints || [], pod.value.gates || []));
const hasConstraints = computed(() => {
  const view = constraintView.value;
  // A gated Pod already shows its gates in the summary.
  const gates = stage.value === 'gated' ? [] : view.gates;
  return Boolean(view.priorityClassName) || gates.length > 0
    || ['nodeSelector', 'device', 'tolerations', 'nodeAffinity', 'podAffinity', 'topology', 'other', 'raw'].some((key) => view[key].length > 0);
});
const rawDefinitions = computed(() => constraintView.value.raw.map((item) => ({ name: item.name, tokens: tokenizeJSON(item.text) })));
const eventStatus = computed(() => ['available', 'empty', 'forbidden', 'timeout', 'limited', 'error'].includes(response.value?.eventStatus) ? response.value.eventStatus : 'error');
const visibleEvents = computed(() => (response.value?.events || []).slice(0, showAllEvents.value ? 50 : 20));
const recordCount = computed(() => (response.value?.events || []).length + (pod.value.condition?.message || pod.value.condition?.reason ? 1 : 0));
const formatDate = (value) => {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US') : '--';
};

// Chinese sentences need no separating space.
const eventFootnote = computed(() => [
  t('scheduling.eventLifetime'),
  response.value?.eventsFetchedAt ? t('scheduling.fetchedAt', { time: formatDate(response.value.eventsFetchedAt) }) : '',
].filter(Boolean).join(locale.value === 'zh' ? '' : ' '));

const onDialogKeydown = (event) => {
  if (event.key !== 'Tab') return;
  const root = content.value?.closest('[role="dialog"]');
  if (!root) return;
  const focusable = [...root.querySelectorAll('button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])')]
    .filter((element) => element.getClientRects().length > 0);
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!first) { event.preventDefault(); root.focus(); return; }
  if (event.shiftKey && (document.activeElement === first || document.activeElement === root)) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
};

const refresh = async () => {
  if (!props.identityPod) return;
  const identity = props.identityPod;
  now.value = Date.now();
  const current = ++generation;
  controller?.abort();
  controller = new AbortController();
  loading.value = true;
  loadError.value = false;
  try {
    const result = await taskApi.getSchedulingDetail({ name: identity.name, namespace: identity.namespace, uid: identity.uid }, controller.signal);
    if (current !== generation) return;
    if (!result?.pod || schedulingIdentity(result.pod) !== schedulingIdentity(identity)) throw new Error('Unexpected Pod identity');
    response.value = result;
    now.value = Date.now();
    identityError.value = '';
    emit('updated', result.pod);
  } catch (error) {
    if (current !== generation) return;
    const kind = getSchedulingDetailError(error);
    if (['gone', 'recreated'].includes(kind)) identityError.value = kind;
    else loadError.value = true;
  } finally {
    if (current === generation) loading.value = false;
  }
};

let copyTimer;
const copyRecord = async (key, record) => {
  clearTimeout(copyTimer);
  // Clear first so a repeated copy is announced again.
  copyStatus.value = '';
  await nextTick();
  try {
    await navigator.clipboard.writeText(JSON.stringify(record, null, 2));
    copyStatus.value = 'copied';
    copiedKey.value = key;
    copyTimer = setTimeout(() => { copiedKey.value = ''; }, 1500);
  } catch {
    copyStatus.value = 'copyFailed';
    copiedKey.value = '';
  }
};

// The button disappears, so focus the first revealed record.
const showMoreEvents = async () => {
  const firstHidden = (pod.value.condition?.message || pod.value.condition?.reason ? 1 : 0) + 20;
  showAllEvents.value = true;
  await nextTick();
  content.value?.querySelectorAll('.sd-record')[firstHidden]?.focus();
};

let hasOpened = false;
watch(() => schedulingIdentity(props.identityPod || {}), () => {
  ++generation;
  controller?.abort();
  response.value = null;
  loading.value = false;
  identityError.value = '';
  loadError.value = false;
  copyStatus.value = '';
  copiedKey.value = '';
  clearTimeout(copyTimer);
  showAllEvents.value = false;
  if (props.identityPod) {
    // Safari and Firefox do not focus a clicked button.
    if (!hasOpened) opener = document.activeElement === document.body ? null : document.activeElement;
    hasOpened = true;
    now.value = Date.now();
    refresh();
    nextTick(() => content.value?.closest('[role="dialog"]')?.focus());
  } else if (hasOpened) {
    hasOpened = false;
    // Run after TDesign's own focus handling on close.
    nextTick(() => requestAnimationFrame(() => {
      if (props.identityPod) return;
      if (opener?.isConnected) opener.focus();
      else props.focusReturnTarget?.querySelector('input, button, [tabindex="0"]')?.focus();
    }));
  }
}, { immediate: true });
onBeforeUnmount(() => { ++generation; controller?.abort(); clearTimeout(copyTimer); });
</script>

<style scoped lang="scss">
$heading: #1d2b3a;
$text: #3b4c5d;
$muted: #939ea9;
$panel: #f5f7fa;
$line: #edf1f5;
$border: #e4ebf1;
$brand: #2563eb;
$mono: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace;

.scheduling-content {
  color: $text;
  font-size: 14px;
  line-height: 1.6;
}

.scheduling-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  font-size: inherit;
  cursor: pointer;

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

// The global stylesheet also styles code.
.sd-chip {
  max-width: 100%;
  padding: 1px 8px;
  border: 1px solid $border;
  border-radius: 6px;
  background: $panel;
  color: $heading;
  font-family: $mono;
  font-size: 12px;
  line-height: 20px;
  overflow-wrap: anywhere;
}

.sd-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.sd-heading {
  min-width: 0;
}

.sd-title-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px 12px;
}

.sd-title {
  margin: 0;
  color: $heading;
  font-size: 18px;
  font-weight: 500;
  line-height: 28px;
  overflow-wrap: anywhere;
}

.sd-state {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #324558;
  line-height: 22px;
  white-space: nowrap;

  :deep(svg) {
    font-size: 16px;
  }
}

.sd-subtitle {
  margin: 2px 0 0;
  color: $muted;
}

.sd-notice {
  margin: 16px 0 0;
  padding: 10px 14px;
  border-radius: 8px;
  background: #fff7eb;
  color: #7a5a1c;
}

.sd-conclusion {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  margin-top: 20px;
  padding: 20px;
  border-radius: 8px;
  background: $panel;
}

.sd-conclusion-main {
  min-width: 0;
}

.sd-headline {
  margin: 0;
  color: $heading;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  text-wrap: pretty;
}

.sd-reasons {
  display: grid;
  gap: 8px;
  margin: 14px 0 0;
  padding: 0;
  list-style: none;

  li {
    display: flex;
    align-items: center;
    gap: 10px;
    color: $heading;
  }
}

.sd-reason-icon {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: #fff;
  box-shadow:
    0 1px 4px 0 rgb(2 5 8 / 6%),
    0 4px 12px 0 rgb(2 5 8 / 6%);
  color: $muted;
  font-size: 16px;
}

.sd-gates {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin: 12px 0 0;
}

.sd-gates-label {
  margin-right: 2px;
  color: $muted;
}

// dt comes first in the DOM but shows below its value.
.sd-facts {
  display: grid;
  align-content: start;
  gap: 14px;
  min-width: 140px;
  max-width: 220px;
  margin: 0 0 0 24px;
  padding-left: 24px;
  border-left: 1px solid $border;

  > div {
    display: flex;
    flex-direction: column-reverse;
    min-width: 0;
  }

  dt {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: $muted;
    font-size: 12px;
    font-weight: 400;
    line-height: 20px;
  }

  dd {
    margin: 0;
    color: $heading;
    font-size: 16px;
    font-weight: 500;
    line-height: 24px;
    overflow-wrap: anywhere;
  }
}

.sd-constraints {
  margin: 0;
  border: 1px solid $border;
  border-radius: 12px;
  overflow: hidden;

  > div {
    display: grid;
    grid-template-columns: 168px minmax(0, 1fr);
    gap: 12px;
    padding: 12px 16px;

    & + div {
      border-top: 1px solid $line;
    }
  }

  dt {
    color: $text;
    font-weight: 400;
    line-height: 24px;
    overflow-wrap: anywhere;
  }

  dd {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    min-width: 0;
    margin: 0;
  }

  .sd-terms {
    flex-direction: column;
    align-items: flex-start;
  }
}

.sd-term {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.sd-term-note {
  color: $muted;
}

.sd-dt-code {
  font-family: $mono;
  font-size: 13px;
}

.sd-note {
  margin: 12px 0 0;
}

.sd-links {
  display: flex;
  flex-direction: column;
  gap: 2px;

  a {
    color: $brand;
  }
}

.sd-section {
  margin-top: 24px;
}

.sd-section-title {
  margin: 0 0 12px;
  color: $heading;
  font-size: 16px;
  font-weight: 500;
  line-height: 20px;
}

.sd-checks {
  margin: 0;
  padding-left: 20px;

  li + li {
    margin-top: 6px;
  }

  li::marker {
    color: $muted;
  }
}

.sd-request + .sd-request {
  margin-top: 16px;
}

.sd-request-name {
  margin: 0 0 8px;
  color: $heading;
  font-weight: 500;
  overflow-wrap: anywhere;
}

.sd-request-kind {
  margin-left: 8px;
  color: $muted;
  font-weight: 400;
}

.sd-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 8px;
}

.sd-card {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
  padding: 15px 16px;
  border-radius: 8px;
  background: $panel;
}

.sd-card-icon {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: #fff;
  box-shadow:
    0 2px 8px 0 rgb(2 5 8 / 4%),
    0 6px 20px 0 rgb(2 5 8 / 8%);
  font-size: 20px;
}

.sd-card-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.sd-card-value {
  color: $heading;
  font-size: 16px;
  font-weight: 500;
  line-height: 28px;
  font-variant-numeric: tabular-nums;

  &.is-unset {
    color: $muted;
    font-weight: 400;
  }
}

.sd-card-label {
  color: $muted;
  font-size: 12px;
  line-height: 20px;
  overflow-wrap: anywhere;
}

.sd-raw {
  margin-top: 12px;

  > summary {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: $brand;
    list-style: none;
    cursor: pointer;

    &::-webkit-details-marker {
      display: none;
    }
  }
}

.sd-code-title {
  margin: 12px 0 6px;
  color: $muted;
  font-family: $mono;
  font-size: 12px;
}

// The global stylesheet also styles pre.
pre.sd-code {
  max-height: 320px;
  margin: 0;
  padding: 12px 16px;
  overflow: auto;
  border-radius: 8px;
  background: $panel;
  color: $heading;
  font-family: $mono;
  font-size: 12px;
  line-height: 1.65;
  white-space: pre;
}

.sd-token-key {
  color: #0550ae;
}

.sd-token-string {
  color: #0a3069;
}

.sd-token-number {
  color: #953800;
}

.sd-token-literal {
  color: #cf222e;
}

.sd-status-line {
  margin: 20px 0 0;
  color: $muted;
}

.sd-records {
  margin-top: 24px;

  > summary {
    display: flex;
    align-items: center;
    gap: 8px;
    list-style: none;
    cursor: pointer;

    &::-webkit-details-marker {
      display: none;
    }

    .sd-section-title {
      margin: 0;
    }
  }
}

.sd-raw > summary::after,
.sd-records > summary::after {
  width: 6px;
  height: 6px;
  margin: 0 0 3px 2px;
  border-right: 1.5px solid currentColor;
  border-bottom: 1.5px solid currentColor;
  content: '';
  transform: rotate(45deg);
  transition: transform 160ms ease;
}

.sd-records > summary::after {
  color: $muted;
}

.sd-raw[open] > summary::after,
.sd-records[open] > summary::after {
  margin-bottom: -3px;
  transform: rotate(-135deg);
}

.sd-raw > summary:focus-visible,
.sd-records > summary:focus-visible {
  outline: 2px solid $brand;
  outline-offset: 2px;
  border-radius: 4px;
}

.sd-records-count {
  color: $muted;
}

.sd-timeline {
  margin: 16px 0 0;
  padding: 0;
  list-style: none;
}

.sd-record {
  position: relative;
  padding: 0 0 20px 22px;

  &::before {
    position: absolute;
    top: 16px;
    bottom: -8px;
    left: 4px;
    width: 1px;
    background: $border;
    content: '';
  }

  &::after {
    position: absolute;
    top: 8px;
    left: 0;
    box-sizing: border-box;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: #c5ced6;
    content: '';
  }

  &:last-child {
    padding-bottom: 4px;

    &::before {
      display: none;
    }
  }

  &.is-warning::after {
    background: #f08c1a;
  }

  &.is-condition::after {
    border: 2px solid $muted;
    background: #fff;
  }
}

.sd-record-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  min-height: 24px;
}

.sd-record-title {
  color: $heading;
  font-weight: 500;
  overflow-wrap: anywhere;
}

.sd-record-source {
  color: $muted;
  overflow-wrap: anywhere;
}

.sd-record-time {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  color: $muted;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.sd-time-separator {
  width: 1px;
  height: 12px;
  margin: 0 6px;
  background: $border;
}

.sd-copy {
  flex: none;
  margin: -4px -4px -4px 0;
}

.sd-more {
  margin-left: 12px;
}

.sd-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}

pre.sd-message {
  margin: 8px 0 0;
  padding: 10px 14px;
  border: 0;
  border-radius: 8px;
  background: $panel;
  color: $heading;
  font-family: $mono;
  font-size: 12px;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.sd-footnote {
  margin: 12px 0 0;
  color: $muted;
  font-size: 12px;
}

// TDesign's loading state would disable the button and drop focus.
.sd-refresh-icon.is-loading {
  animation: sd-spin 0.9s linear infinite;
}

@keyframes sd-spin {
  to {
    transform: rotate(360deg);
  }
}

.sd-record:focus-visible {
  outline: 2px solid $brand;
  outline-offset: 4px;
  border-radius: 4px;
}

@media (prefers-reduced-motion: reduce) {
  .sd-raw > summary::after,
  .sd-records > summary::after {
    transition: none;
  }

  .sd-refresh-icon.is-loading {
    animation: none;
    opacity: 0.5;
  }
}

@media (max-width: 540px) {
  .sd-conclusion {
    grid-template-columns: minmax(0, 1fr);
  }

  .sd-facts {
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    max-width: none;
    margin: 16px 0 0;
    padding: 16px 0 0;
    border-top: 1px solid $border;
    border-left: 0;
  }

  .sd-constraints > div {
    grid-template-columns: minmax(0, 1fr);
    gap: 4px;
  }
}
</style>

<style lang="scss">
// The panel is outside this component's scoped styles.
.scheduling-drawer .t-drawer__content-wrapper--right {
  border-radius: 12px 0 0 12px;

  @media (max-width: 680px) {
    border-radius: 0;
  }
}
</style>
