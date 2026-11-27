<template>
  <t-config-provider :global-config="tdGlobalConfig">
    <router-view></router-view>
  </t-config-provider>
</template>

<script setup>
import { computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { ConfigProvider as TConfigProvider } from 'tdesign-vue-next';
import tdesignEnUS from 'tdesign-vue-next/es/locale/en_US';
import tdesignZhCN from 'tdesign-vue-next/es/locale/zh_CN';
import { toDocumentLanguage } from '@/locales/language.mjs';

defineOptions({
  name: 'App',
});

const { locale } = useI18n();

const tdGlobalConfig = computed(() =>
  locale.value === 'en' ? tdesignEnUS : tdesignZhCN,
);

watch(
  locale,
  (language) => {
    document.documentElement.lang = toDocumentLanguage(language);
  },
  { immediate: true },
);
</script>
