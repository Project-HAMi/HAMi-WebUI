<template>
  <router-link
    class="navbar__brand"
    :class="{ 'navbar__brand--collapsed': collapsed }"
    to="/"
    aria-label="HAMi WebUI"
  >
    <img src="@/assets/logo/hami-graph-color.svg" alt="" class="navbar__icon" />
    <b class="navbar__title" aria-hidden="true">HAMi WebUI</b>
  </router-link>
</template>

<script setup>
defineProps({
  collapsed: {
    type: Boolean,
    default: false,
  },
});
</script>

<style scoped>
.navbar__brand {
  display: flex;
  align-items: center;
  flex: 0 0 auto;
  gap: 10px;
  width: 160px;
  height: 32px;
  overflow: hidden;
  border-radius: 4px;
  text-decoration: none;
  transition: width 180ms cubic-bezier(0.2, 0, 0, 1);
}

.navbar__brand--collapsed {
  width: 32px;
}

.navbar__brand:focus-visible {
  outline: 2px solid var(--td-brand-color);
  outline-offset: 4px;
}

.navbar__icon {
  display: block;
  flex: 0 0 32px;
  width: 32px;
  height: 32px;
}

.navbar__title {
  flex-shrink: 0;
  font-family: -apple-system, BlinkMacSystemFont, var(--td-font-family);
  font-size: 16px;
  font-weight: 600;
  line-height: 20px;
  color: #203040;
  white-space: nowrap;
  transition: opacity 120ms ease;
}

.navbar__brand--collapsed .navbar__title {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .navbar__brand,
  .navbar__title {
    transition: none;
  }
}
</style>
