<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const time = defineModel();

const { t } = useI18n();

const timeOptions = computed(() => [
  { label: t('timeRange.last7d'), value: 7 },
  { label: t('timeRange.last5d'), value: 5 },
  { label: t('timeRange.last3d'), value: 3 },
  { label: t('timeRange.last24h'), value: 1 },
  { label: t('timeRange.last12h'), value: 0.5 },
  { label: t('timeRange.last1h'), value: 0.5 / 12 },
]);
</script>

<template>
  <el-select v-model="time" style="width: 150px">
    <el-option
      v-for="{ label, value } in timeOptions"
      :key="value"
      :label="label"
      :value="value"
    ></el-option>
  </el-select>
</template>

<style scoped lang="scss"></style>
