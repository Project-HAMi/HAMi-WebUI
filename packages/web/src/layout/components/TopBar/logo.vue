<template>
  <router-link
    class="navbar__brand"
    :class="{ 'navbar__brand--collapsed': collapsed }"
    to="/"
    aria-label="HAMi WebUI"
  >
    <span class="navbar__art" aria-hidden="true" v-html="brandSvg" />
  </router-link>
</template>

<script setup>
// Share the complete, local SVG with documentation; keep its geometry intact.
import brandSvg from '@brand/hami-webui-horizontal-light.svg?raw';

defineProps({
  collapsed: {
    type: Boolean,
    default: false,
  },
});
</script>

<style scoped>
.navbar__brand {
  position: relative;
  display: block;
  flex: 0 0 auto;
  width: 148px;
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

.navbar__art {
  position: absolute;
  top: 0;
  /* Center the SVG's 111.018937-unit mark in the stable 32px icon column. */
  left: 2.122633px;
  display: block;
  width: 145.5px;
  height: 32px;
}

.navbar__art :deep(svg) {
  display: block;
  width: 100%;
  height: 100%;
}

.navbar__art :deep(.hami-webui-lettering) {
  opacity: 1;
  transition: opacity 120ms ease;
}

.navbar__brand--collapsed :deep(.hami-webui-lettering) {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .navbar__brand,
  .navbar__art :deep(.hami-webui-lettering) {
    transition: none;
  }
}
</style>
