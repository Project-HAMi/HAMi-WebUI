<template>
  <div v-if="message" class="device-config-alert" role="status">
    <svg-icon icon="alarm-warning" aria-hidden="true" />
    <span>{{ message }}</span>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import apiClient from '@/api/client.mjs';
import { getDeviceConfigStateKey, shouldWarnAboutDeviceConfig } from '~/vgpu/views/card/admin/device-config-display.mjs';

const { t, locale } = useI18n();
const config = ref(null);
const deviceTypes = ref([]);
let controller = null;

onMounted(async () => {
  const currentController = new AbortController();
  controller = currentController;
  try {
    const read = await apiClient.getDeviceConfig({ signal: currentController.signal });
    if (currentController.signal.aborted) return;
    config.value = read;
  } catch {
    return;
  }
  if (!config.value?.state || config.value.state === 'loaded' || config.value.state === 'loading') {
    return;
  }
  try {
    const types = await apiClient.listDevices({}, { signal: currentController.signal });
    if (currentController.signal.aborted) return;
    deviceTypes.value = (types?.list || []).map((item) => item?.type).filter(Boolean);
  } catch {
    // Ignore aborted or failed registration lookup; alert stays hidden without confirmed Ascend types.
  }
});

onUnmounted(() => {
  controller?.abort();
  controller = null;
});

const message = computed(() => {
  if (!shouldWarnAboutDeviceConfig(config.value, deviceTypes.value)) return '';
  const state = t(getDeviceConfigStateKey(config.value), {
    namespace: config.value.namespace || '--',
    name: config.value.name || '--',
  });
  // Chinese sentences need no separating space.
  return [state, t('card.deviceConfig.impact')].join(locale.value === 'zh' ? '' : ' ');
});
</script>

<style lang="scss" scoped>
.device-config-alert {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 8px;
  background: #fff7eb;
  color: #7a5a1c;
  font-size: 13px;
  line-height: 20px;

  svg {
    flex-shrink: 0;
    width: 18px;
    height: 18px;
  }
}
</style>
