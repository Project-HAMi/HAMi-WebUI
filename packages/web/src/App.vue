<template>
  <el-config-provider :locale="elLocale">
    <t-config-provider :global-config="tdGlobalConfig">
      <router-view></router-view>
    </t-config-provider>
  </el-config-provider>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ElConfigProvider } from 'element-plus';
import { ConfigProvider as TConfigProvider } from 'tdesign-vue-next';
import tdesignEnUS from 'tdesign-vue-next/es/locale/en_US';
import tdesignZhCN from 'tdesign-vue-next/es/locale/zh_CN';
import zhCn from 'element-plus/es/locale/lang/zh-cn';
import en from 'element-plus/es/locale/lang/en';

defineOptions({
  name: 'App',
});

const { locale } = useI18n();

const elLocale = computed(() => {
  return locale.value === 'zh' ? zhCn : en;
});

const tdGlobalConfig = computed(() =>
  locale.value === 'en' ? tdesignEnUS : tdesignZhCN,
);
</script>
