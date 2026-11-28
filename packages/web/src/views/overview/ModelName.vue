<template>
  <RouterLink v-if="row.type !== null" class="model-name" :to="{ name: 'accelerators', query: { type: row.type } }">
    <VendorMark :vendor="row.vendor" />
    <span>{{ row.type }}</span>
  </RouterLink>
  <span v-else>{{ unknown }}</span>
</template>

<script setup>
import VendorMark from '../../components/VendorMark.vue';
import { RouterLink } from 'vue-router';
defineProps({
  row: { type: Object, required: true },
  unknown: { type: String, required: true },
});
</script>

<style scoped>
.model-name {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  vertical-align: middle;
  color: inherit;
  text-decoration: none;
}
.model-name span { min-width: 0; overflow-wrap: anywhere; }
.model-name:hover { color: var(--td-brand-color); text-decoration: underline; text-underline-offset: 3px; }
.model-name:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: 3px; border-radius: 2px; }
</style>
