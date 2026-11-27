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

const props = defineProps({
  deviceTypes: {
    type: Array,
    default: null,
  },
});

const { t, locale } = useI18n();
const config = ref(null);
const fetchedDeviceTypes = ref([]);
let controller = null;

onMounted(async () => {
  controller = new AbortController();
  const requests = [apiClient.getDeviceConfig({ signal: controller.signal })];
  if (props.deviceTypes === null) {
    requests.push(apiClient.listDevices({}, { signal: controller.signal }));
  }
  const [read, types] = await Promise.allSettled(requests);
  if (controller.signal.aborted) return;
  if (read.status === 'fulfilled') config.value = read.value;
  if (types?.status === 'fulfilled') {
    fetchedDeviceTypes.value = (types.value?.list || []).map((item) => item?.type).filter(Boolean);
  }
});

onUnmounted(() => {
  controller?.abort();
  controller = null;
});

const effectiveDeviceTypes = computed(() => props.deviceTypes ?? fetchedDeviceTypes.value);

const message = computed(() => {
  if (!shouldWarnAboutDeviceConfig(config.value, effectiveDeviceTypes.value)) return '';
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
