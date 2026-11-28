<template>
  <div class="device-facts" :aria-busy="loading">
    <div class="device-facts__grid">
      <div class="fact-identities">
        <dl class="fact fact--model">
          <dt>{{ modelLabel }}</dt>
          <dd>
            <LoadingValue v-if="loading" :height="24" width="160px" />
            <template v-else>
              <span class="fact__model">
                <VendorMark v-if="device.type" :vendor="device.vendor">
                  <svg-icon :icon="fallbackModelIcon" aria-hidden="true" />
                </VendorMark>
                <span>{{ device.type || '--' }}</span>
              </span>
              <UnconfiguredTag v-if="device.unconfigured" />
            </template>
          </dd>
        </dl>
        <dl class="fact fact--node">
          <dt>{{ t('card.node') }}</dt>
          <dd>
            <LoadingValue v-if="loading" :height="24" width="100px" />
            <RouterLink v-else-if="nodeLocation && device.nodeName" :to="nodeLocation" class="fact__node-link">
              <span>{{ device.nodeName }}</span><svg-icon icon="jump" aria-hidden="true" />
            </RouterLink>
            <template v-else>{{ device.nodeName || '--' }}</template>
          </dd>
        </dl>
      </div>
      <dl class="fact fact--mode">
        <dt>{{ t('card.splitMode.label') }}</dt>
        <dd>
          <LoadingValue v-if="loading" :height="24" width="80px" />
          <span v-else class="fact__mode">
            <svg-icon v-if="modeIcon" :icon="modeIcon" aria-hidden="true" />
            <span>{{ mode }}</span>
          </span>
        </dd>
      </dl>
      <div class="fact fact--sensors">
        <dl v-for="reading in readings" :key="reading.key" class="fact__sensor">
          <dt>{{ t(reading.key) }}</dt>
          <dd>
            <LoadingValue v-if="loading || reading.loading" :height="24" width="50px" />
            <template v-else>{{ reading.text }}</template>
          </dd>
        </dl>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { RouterLink } from 'vue-router';
import { useI18n } from 'vue-i18n';
import LoadingValue from '@/components/LoadingValue.vue';
import VendorMark from '@/components/VendorMark.vue';
import UnconfiguredTag from './UnconfiguredTag.vue';
import { deviceWording } from '@/components/device-copy.mjs';

const props = defineProps({
  loading: Boolean,
  device: { type: Object, default: () => ({}) },
  nodeLocation: Object,
  mode: String,
  modeIcon: String,
  readings: { type: Array, required: true },
});
const { t } = useI18n();
const fallbackModelIcon = computed(() => /^AWS(?:NEURON)?(?:[^a-z]|$)/i.test(props.device.type || '') ? 'gpu-aws' : 'vgpu-card');
const modelLabel = computed(() => deviceWording(t('card.model'), props.device.vendor));
</script>

<style scoped lang="scss">
.device-facts { container: device-facts / inline-size; }
.device-facts__grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(11.5rem, .23fr) minmax(12rem, .31fr);
  gap: 8px;
  margin: 0;
}
.fact-identities { display: flex; min-width: 0; gap: 8px; }
.fact {
  display: flex;
  flex-direction: column-reverse;
  justify-content: center;
  gap: 3px;
  min-width: 0;
  margin: 0;
  padding: 12px 16px;
  background: #f5f7fa;
  border-radius: 8px;
}
.fact-identities .fact { flex: 1 1 auto; max-inline-size: 65%; }
.fact dt { font-size: 12px; font-weight: 400; line-height: 18px; color: #697886; }
.fact dd {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 6px;
  min-width: 0;
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  color: #324558;
  overflow-wrap: anywhere;
}
.fact__model, .fact__node-link, .fact__mode {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  min-width: 0;
}
.fact__model > span, .fact__node-link > span, .fact__mode > span { min-width: 0; }
.fact :deep(svg) { width: 16px; height: 16px; flex-shrink: 0; }
.fact__node-link {
  color: inherit;
  border-radius: 3px;
  text-decoration: none;
  &:hover, &:focus-visible { color: var(--td-brand-color); }
  &:hover span, &:focus-visible span { text-decoration: underline; text-underline-offset: 3px; }
  &:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: 2px; }
}
.fact--sensors {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  align-items: center;
  gap: 12px;
}
.fact__sensor {
  display: flex;
  flex-direction: column-reverse;
  gap: 3px;
  min-width: 0;
  margin: 0;
  font-variant-numeric: tabular-nums;
}
@container device-facts (max-width: 799px) {
  .device-facts__grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .fact-identities { display: contents; }
  .fact-identities .fact { max-inline-size: none; }
}
@container device-facts (max-width: 479px) {
  .fact--model, .fact--sensors { grid-column: 1 / -1; }
  .fact { padding: 10px 12px; }
}
@container device-facts (max-width: 359px) {
  .device-facts__grid { grid-template-columns: minmax(0, 1fr); }
  .fact--sensors { grid-template-columns: repeat(auto-fit, minmax(min(100%, 68px), 1fr)); }
}
</style>
