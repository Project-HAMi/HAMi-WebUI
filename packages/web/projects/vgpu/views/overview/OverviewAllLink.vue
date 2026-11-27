<template>
  <RouterLink class="overview-all-link" :to="to" :aria-label="label">
    <span>{{ $t('dashboard.viewAll') }}</span>
    <SvgIcon icon="more" />
  </RouterLink>
</template>

<script setup>
import { RouterLink } from 'vue-router';
import SvgIcon from '@/components/SvgIcon/index.vue';

defineProps({
  to: { type: [String, Object], required: true },
  label: { type: String, required: true },
});
</script>

<style scoped>
.overview-all-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 24px;
  color: #697886;
  font-size: 12px;
  line-height: 20px;
  text-decoration: none;
  white-space: nowrap;
}

.overview-all-link:hover {
  color: var(--td-brand-color);
}

.overview-all-link:focus-visible {
  outline: 2px solid var(--td-brand-color);
  outline-offset: 2px;
  border-radius: 4px;
}
</style>
