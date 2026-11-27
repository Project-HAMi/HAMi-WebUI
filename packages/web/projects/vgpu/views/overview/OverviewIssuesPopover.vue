<template>
  <t-popup
    v-if="view === 'details'"
    v-model:visible="open"
    trigger="click"
    placement="bottom-right"
    :popper-options="popperOptions"
    overlay-class-name="overview-issues-popup"
    overlay-inner-class-name="overview-issues-surface"
  >
    <button
      ref="trigger"
      type="button"
      class="issues-trigger"
      :aria-label="panelTitle === label ? label : `${panelTitle} · ${label}`"
      :title="label"
      :aria-expanded="open"
      :aria-controls="open ? panelId : undefined"
      aria-haspopup="dialog"
      @keydown.esc="dismiss"
    >
      <svg-icon icon="status-unschedulable" aria-hidden="true" />
      <span class="issues-trigger__label">{{ label }}</span>
      <span v-if="count" class="issues-trigger__compact" aria-hidden="true">{{ count }}</span>
      <ChevronDownIcon class="issues-trigger__chevron" aria-hidden="true" />
    </button>
    <template #content>
      <section
        v-if="open"
        :id="panelId"
        ref="panel"
        class="issues-panel"
        role="dialog"
        :aria-labelledby="titleId"
        tabindex="-1"
        @keydown.esc.stop.prevent="dismiss"
        @keydown.tab="handleTab"
      >
        <div class="issues-panel__header">
          <h2 :id="titleId">{{ panelTitle }}</h2>
          <button type="button" class="issues-panel__close" :aria-label="t('close')" @click="dismiss">
            <CloseIcon aria-hidden="true" />
          </button>
        </div>
        <NodeDeviceIssues :summary="summary" status="ready" />
      </section>
    </template>
  </t-popup>
  <t-tooltip v-else-if="view === 'ready'" overlay-class-name="overview-healthy-tooltip" :content="t('healthy')" :visible="hintHovered || hintFocused" placement="bottom-right">
    <span
      ref="staticStatus"
      class="issues-healthy"
      role="img"
      tabindex="0"
      :aria-label="t('healthy')"
      @mouseenter="hintHovered = true"
      @mouseleave="hintHovered = false"
      @focus="hintFocused = true"
      @blur="hintFocused = false"
      @keydown.esc="clearHint"
    >
      <CheckCircleIcon aria-hidden="true" />
    </span>
  </t-tooltip>
  <span v-else-if="status !== 'empty'" ref="staticStatus" class="issues-state" role="status" tabindex="-1">
    {{ status === 'loading' ? t('loading') : t('unavailable') }}
  </span>
</template>

<script setup>
import { computed, nextTick, ref, useId, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { CheckCircleIcon, ChevronDownIcon, CloseIcon } from 'tdesign-icons-vue-next';
import NodeDeviceIssues from './NodeDeviceIssues.vue';
import { summarizeNodeIssues } from './node-device-issues.mjs';

const emit = defineEmits(['details-visible']);
const props = defineProps({
  issues: { type: Array, default: () => [] },
  status: { type: String, default: 'loading' },
});
const { t: translate } = useI18n();
const t = (key, ...args) => translate(`dashboard.issueSummary.${key}`, ...args);
const summary = computed(() => summarizeNodeIssues(props.issues));
const count = computed(() => summary.value.incidents.length);
const hasRows = computed(() => count.value > 0);
const view = computed(() => hasRows.value ? 'details' : props.status);
const panelTitle = computed(() => translate('dashboard.nodeDeviceIssues'));
const label = computed(() => t('count', count.value));
const hintHovered = ref(false);
const hintFocused = ref(false);
function clearHint() {
  hintHovered.value = false;
  hintFocused.value = false;
}
const panelId = useId();
const titleId = `${panelId}-title`;
const popperOptions = {
  strategy: 'fixed',
  modifiers: [
    { name: 'preventOverflow', options: { padding: 8 } },
    {
      name: 'availableHeight',
      enabled: true,
      phase: 'beforeWrite',
      requires: ['computeStyles'],
      fn({ state }) {
        const anchor = state.elements.reference.getBoundingClientRect();
        const room = state.placement.startsWith('top') ? anchor.top : document.documentElement.clientHeight - anchor.bottom;
        state.elements.popper.style.setProperty('--issues-available-height', `${Math.max(0, room - 18)}px`);
      },
    },
  ],
};
const open = ref(false);
watch(open, visible => emit('details-visible', visible));
const panel = ref(null);
const trigger = ref(null);
const staticStatus = ref(null);
watch(view, async () => {
  clearHint();
  const previous = trigger.value ?? staticStatus.value;
  const restoreFocus = previous === document.activeElement || panel.value?.contains(document.activeElement);
  const header = previous?.closest('header');
  open.value = false;
  if (!restoreFocus) return;
  // A refresh can replace the focused control, or remove it for an empty inventory.
  await nextTick();
  const next = trigger.value ?? staticStatus.value ?? header;
  next?.focus({ preventScroll: true });
});
watch(panel, (element) => element?.focus({ preventScroll: true }), { flush: 'post' });

function dismiss() {
  if (!open.value) return;
  open.value = false;
  trigger.value?.focus({ preventScroll: true });
}

function handleTab(event) {
  // The popup is attached to body. Resume the page's tab order at its trigger.
  const stops = Array.from(panel.value.querySelectorAll('button:enabled, a[href], summary, [tabindex="0"]')).filter(element => element.getClientRects().length);
  if (event.shiftKey && (event.target === panel.value || event.target === stops[0])) {
    event.preventDefault();
    dismiss();
  } else if (!event.shiftKey && event.target === stops[stops.length - 1]) {
    dismiss();
  }
}
</script>

<style scoped>
.issues-trigger,
.issues-panel__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: #576777;
  cursor: pointer;
}
.issues-trigger {
  min-height: 32px;
  padding: 0 6px;
  font-family: inherit;
  font-size: 13px;
  font-weight: 500;
  line-height: 1.4;
  max-inline-size: 100%;
  text-align: start;
  overflow-wrap: anywhere;
}
.issues-trigger .t-icon,
.issues-trigger .svg-icon { flex-shrink: 0; }
.issues-trigger .svg-icon { font-size: 16px; }
.issues-trigger__compact { display: none; }
.issues-healthy {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 32px;
  color: #697886;
}
:global(.overview-healthy-tooltip) { --td-comp-margin-l: 4px; }
.issues-healthy .t-icon { font-size: 16px; }
.issues-state { overflow-wrap: anywhere; padding-inline: 6px; color: #697886; font-size: 13px; font-weight: 400; }
@container overview (max-width: 160px) {
  .issues-trigger { gap: 3px; padding-inline: 3px; }
  .issues-trigger .issues-trigger__compact { display: inline-flex; white-space: nowrap; }
  .issues-trigger__label { display: none; }
  .issues-state { padding-inline: 0; line-height: 1.4; }
}
.issues-trigger:hover,
.issues-trigger[aria-expanded='true'],
.issues-panel__close:hover { background: #f1f4f7; }
.issues-trigger:focus-visible,
.issues-healthy:focus-visible,
.issues-panel__close:focus-visible {
  outline: 2px solid #2563eb;
  outline-offset: 1px;
}
.issues-trigger .t-icon { font-size: 15px; }
.issues-trigger .issues-trigger__chevron { font-size: 12px; }
.issues-trigger[aria-expanded='true'] .issues-trigger__chevron { transform: rotate(180deg); }
/* Let keyboard focus enter the dialog as its opening fade starts. */
:global(.overview-issues-popup.t-popup--animation-enter-from) { visibility: visible; }
:global(.t-popup__content.overview-issues-surface) {
  border: 1px solid #dce4eb;
  border-radius: 10px;
  padding: 0;
  box-shadow: 0 6px 24px rgb(32 48 64 / 12%);
}
.issues-panel {
  --issue-inline-inset: 16px;
  box-sizing: border-box;
  padding-block: 12px;
  display: flex;
  flex-direction: column;
  width: min(472px, calc(100vw - 20px));
  max-height: min(560px, var(--issues-available-height, calc(100vh - 90px)));
  overflow: hidden;
  overscroll-behavior: contain;
  outline: 0;
  word-break: normal;
  scrollbar-width: thin;
}
.issues-panel__header {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 0 var(--issue-inline-inset) 8px;
  background: #fff;
  border-radius: 10px 10px 0 0;
}
.issues-panel > :deep(.node-issues) { flex: 1; min-height: 0; }
.issues-panel h2 {
  margin: 0;
  color: #1d2b3a;
  font-size: 15px;
  font-weight: 500;
  line-height: 20px;
}
.issues-panel__close { width: 28px; height: 28px; font-size: 16px; }
@media (max-width: 480px) {
  .issues-panel { --issue-inline-inset: 12px; }
}
@media (max-height: 500px) {
  .issues-panel { padding-block: 8px; }
}
</style>
