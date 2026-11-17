<template>
  <section class="report-draft" @keydown.esc.capture.stop.prevent="onEscape">
    <t-button class="report-back" variant="text" theme="default" type="button" @click="emit('close')">
      <template #icon><ChevronLeftIcon aria-hidden="true" /></template>
      {{ t('issueDraft.back') }}
    </t-button>
    <div class="report-field">
      <label>{{ t('issueDraft.component') }}
      <t-select
        ref="select"
        v-model="selectedId"
        v-model:popup-visible="menuOpen"
        :options="components"
        :keys="{ label: 'label', value: 'id' }"
        :popup-props="{ overlayClassName: 'select-option-label-popup' }"
        filterable
      />
      </label>
    </div>
    <div v-if="repository" class="report-repo">
      <LogoGithubIcon aria-hidden="true" />
      <span><small>{{ t('issueDraft.destination') }}</small>{{ repository }}</span>
    </div>
    <p v-else-if="selected" class="report-hint">{{ t('issueDraft.destinationUnknown') }}</p>
    <div v-if="selected" class="report-summary">
      <label v-if="repository"><input v-model="includeSummary" type="checkbox" />{{ t('issueDraft.include') }}</label>
      <span v-else>{{ t('issueDraft.preview') }}</span>
      <textarea
        v-if="includeSummary || !repository"
        ref="summaryInput"
        v-model="summary"
        :aria-label="t('issueDraft.edit')"
        :placeholder="t('issueDraft.noInfo')"
        rows="4"
        maxlength="1000"
      />
    </div>
    <div v-if="selected" class="report-actions">
      <a v-if="repository" :href="issueHref" target="_blank" rel="noopener noreferrer">
        {{ t('issueDraft.open') }}<JumpIcon aria-hidden="true" />
      </a>
      <button v-else type="button" @click="copy">{{ t('issueDraft.copy') }}</button>
      <span role="status">{{ feedback }}</span>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ChevronLeftIcon, JumpIcon, LogoGithubIcon } from 'tdesign-icons-vue-next';

const props = defineProps({
  environment: { type: String, required: true },
  components: { type: Array, required: true },
  initialId: { type: String, default: 'webui' },
});
const emit = defineEmits(['close']);
const { t } = useI18n();

const select = ref(null);
const menuOpen = ref(false);
function onEscape() {
  if (menuOpen.value) menuOpen.value = false;
  else emit('close');
}
const selectedId = ref(props.components.find(item => item.id === props.initialId)?.id);
const selected = computed(() => props.components.find(item => item.id === selectedId.value));
// The form prepares GitHub issues only for active public repositories in the catalog.
const repository = computed(() => {
  const href = selected.value?.issueRepository;
  return href?.match(/^https:\/\/github\.com\/([\w.-]+\/[\w.-]+)$/)?.[1];
});
const includeSummary = ref(true);
const edits = ref({});
const feedback = ref('');
const summaryInput = ref(null);
const summary = computed({
  get() {
    const item = selected.value;
    if (!item) return '';
    const edited = edits.value[item.id];
    if (edited !== undefined) return edited;
    if (item.id === 'webui') return props.environment;
    const details = [item.version, item.readySummary].filter(Boolean).join(', ');
    const name = item.reportName || item.label;
    const heading = details ? `${name}: ${details}` : name;
    return [heading, props.environment].filter(Boolean).join('\n');
  },
  set(value) { edits.value[selectedId.value] = value; },
});
const issueHref = computed(() => {
  if (!repository.value) return '';
  const url = new URL('https://github.com/' + repository.value + '/issues/new');
  const sections = ['Component: ' + (selected.value.reportName || selected.value.label), '## What happened?', ''];
  if (includeSummary.value && summary.value.trim()) sections.push('## Environment', summary.value.trim());
  url.searchParams.set('body', sections.join('\n\n'));
  return url.href;
});
watch(selected, item => {
  if (!item) selectedId.value = undefined;
});
watch(selectedId, () => { feedback.value = ''; });
onMounted(() => select.value?.$el?.querySelector('input')?.focus());
async function copy() {
  const input = summaryInput.value;
  if (!input) return;
  const componentId = selectedId.value;
  try {
    await navigator.clipboard.writeText(input.value);
    if (selectedId.value !== componentId) return;
    feedback.value = t('issueDraft.copied');
  } catch {
    if (selectedId.value !== componentId || !input.isConnected) return;
    input.focus();
    input.select();
    feedback.value = t('issueDraft.copyFailed');
  }
}
</script>

<style scoped>
.report-draft { display: grid; gap: 16px; width: min(100%, 560px); margin: 0 auto; padding: 4px 0 10px; color: var(--env-ink); }
.report-actions a, .report-actions button { display: inline-flex; align-items: center; gap: 6px; min-height: 32px; border: 0; font: inherit; cursor: pointer; }
.report-back {
  justify-self: start;
  margin-inline-start: -8px;
  padding-inline: 4px 8px;
}
.report-back :deep(.t-icon + .t-button__text:not(:empty)) { margin-inline-start: 4px; }
.report-back:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: 2px; }
.report-draft svg { width: 16px; height: 16px; flex-shrink: 0; }
.report-field, .report-summary { display: grid; gap: 8px; font-weight: 500; }
.report-field label { display: grid; gap: 8px; }
.report-summary textarea { box-sizing: border-box; width: 100%; border: 1px solid #cbd5df; border-radius: 6px; background: white; color: inherit; padding: 8px 10px; font: inherit; }
.report-summary textarea { resize: vertical; min-height: 76px; line-height: 1.5; font-weight: 400; }
.report-repo { display: flex; align-items: center; gap: 10px; }
.report-repo small { display: block; font-size: 12px; color: var(--env-muted); }
.report-repo span { overflow-wrap: anywhere; }
.report-summary label { display: flex; align-items: center; gap: 8px; cursor: pointer; }
.report-summary input { margin: 0; accent-color: var(--td-brand-color); }
.report-actions { display: flex; align-items: center; gap: 10px; }
.report-actions a, .report-actions button { background: var(--td-brand-color); color: white; border-radius: 6px; padding: 0 12px; text-decoration: none; }
.report-actions a:hover, .report-actions button:hover { background: var(--td-brand-color-hover); }
.report-actions span, .report-hint { color: var(--env-muted); }
</style>
