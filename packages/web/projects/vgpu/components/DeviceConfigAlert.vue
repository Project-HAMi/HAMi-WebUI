<template>
  <div v-if="message" class="device-config-alert" role="status">
    <svg-icon icon="alarm-warning" aria-hidden="true" />
    <span>{{ message }}</span>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import cardApi from '~/vgpu/api/card';
import deviceConfigApi from '~/vgpu/api/deviceConfig';
import { getDeviceConfigStateKey, shouldWarnAboutDeviceConfig } from '~/vgpu/views/card/admin/device-config-display.mjs';

const { t, locale } = useI18n();
const config = ref(null);
const deviceTypes = ref([]);

onMounted(async () => {
  const [read, types] = await Promise.allSettled([
    deviceConfigApi.getDeviceConfig(),
    cardApi.getCardTypeReq({ filters: {} }),
  ]);
  if (read.status === 'fulfilled') config.value = read.value;
  if (types.status === 'fulfilled') {
    deviceTypes.value = (types.value?.list || []).map((item) => item?.type).filter(Boolean);
  }
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
