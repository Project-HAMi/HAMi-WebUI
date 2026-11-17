<template>
  <div :class="['sidebar', { 'is-collapsed': collapsed }]">
    <div class="side-logo">
      <Logo :collapsed="collapsed" />
    </div>

    <nav id="main-navigation" class="side-menus" :aria-label="$t('common.mainNavigation')">
      <t-tooltip
        v-for="item in navigationItems"
        :key="item.path"
        :content="$t(item.meta.title)"
        :disabled="!collapsed"
        placement="right"
      >
        <router-link
          :to="item.path"
          class="side-link"
          :class="{ 'is-active': activeValue === item.path }"
          :aria-label="$t(item.meta.title)"
          :aria-current="activeValue === item.path ? 'page' : undefined"
        >
          <span class="side-icon">
            <svg-icon :icon="item.meta.icon" />
          </span>
          <span class="side-label" :aria-hidden="collapsed">{{ $t(item.meta.title) }}</span>
        </router-link>
      </t-tooltip>
    </nav>

    <div class="side-footer">
      <LangSelect sidebar :collapsed="collapsed" />
      <t-tooltip
        :content="$t(collapsed ? 'common.expandSidebar' : 'common.collapseSidebar')"
        :disabled="!collapsed"
        placement="right"
      >
        <button
          type="button"
          class="side-toggle"
          :aria-label="$t(collapsed ? 'common.expandSidebar' : 'common.collapseSidebar')"
          :aria-expanded="!collapsed"
          aria-controls="main-navigation"
          @click="$emit('toggle')"
        >
          <span class="side-icon">
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
              <path d="M9 4.5v15" />
              <path :d="collapsed ? 'm13 9 3 3-3 3' : 'm16 9-3 3 3 3'" />
            </svg>
          </span>
          <span class="side-label" :aria-hidden="collapsed">{{ $t('common.collapseSidebar') }}</span>
        </button>
      </t-tooltip>
      <t-tooltip :content="$t('about.title')" :disabled="!collapsed" placement="right">
        <button ref="aboutButton" type="button" class="side-about" :aria-label="$t('about.title')" aria-haspopup="dialog" @click="aboutOpen = true">
          <span class="side-icon">
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <circle cx="12" cy="12" r="8.5" />
              <path d="M12 11v5m0-9v.2" />
            </svg>
          </span>
          <span class="side-label" :aria-hidden="collapsed">{{ $t('about.navLabel') }}</span>
        </button>
      </t-tooltip>
    </div>
    <AboutPanel :visible="aboutOpen" @close="closeAbout" />
  </div>
</template>

<script setup>
import { computed, nextTick, ref } from 'vue';
import { useRoute } from 'vue-router';
import AboutPanel from '~/vgpu/components/AboutPanel.vue';
import Logo from '@/layout/components/TopBar/logo.vue';
import LangSelect from '@/components/LangSelect/index.vue';

defineProps({
  collapsed: {
    type: Boolean,
    default: false,
  },
});

defineEmits(['toggle']);

const aboutOpen = ref(false);
const aboutButton = ref(null);
const closeAbout = async () => {
  aboutOpen.value = false;
  await nextTick();
  requestAnimationFrame(() => aboutButton.value?.focus({ preventScroll: true }));
};

const route = useRoute();

const navigationItems = computed(() =>
  (route.matched[0]?.children ?? []).filter((item) => item.meta),
);

const activeValue = computed(() => {
  const matches = navigationItems.value.filter(
    (item) => route.path === item.path || route.path.startsWith(`${item.path}/`),
  );
  matches.sort((a, b) => b.path.length - a.path.length);
  return matches[0]?.path ?? route.path;
});
</script>

<style lang="scss" scoped>
@import './menu.scss';
</style>
