<template>
  <div class="page">
    <aside
      v-if="!isNoSidebar"
      :style="{ width: sidebarWidth + 'px' }"
      class="page-aside"
    >
      <Sidebar
        :collapsed="isSidebarCollapsed"
        @toggle="toggleSidebar"
      />
    </aside>
    <main class="page-main">
      <AppMain :show-language-switch="isNoSidebar" />
    </main>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import { AppMain, Sidebar } from './components';

const route = useRoute();

const expandedWidth = 200;
const collapsedWidth = 72;
const isSidebarCollapsed = ref(false);

const sidebarWidth = computed(() =>
  isSidebarCollapsed.value ? collapsedWidth : expandedWidth,
);

const toggleSidebar = () => {
  isSidebarCollapsed.value = !isSidebarCollapsed.value;
};

const noSidebarPaths = [
  '/admin/home',
  '/admin/message-center',
  '/admin/about-system',
  '/admin/settings/config-map',
];

const isNoSidebar = computed(() => noSidebarPaths.includes(route.fullPath));
</script>

<style lang="scss" scoped>
.page {
  --app-frame-inset: 8px;
  position: relative;
  display: flex;
  flex-direction: row;
  padding: 0;
  height: 100vh;
  box-sizing: border-box;
  overflow: hidden;
}

.page-aside {
  margin: 0;
  position: relative;
  flex-shrink: 0;
  padding: 0;
  background-color: transparent;
  border: none;
  overflow: hidden;
  transition: width 180ms cubic-bezier(0.2, 0, 0, 1);
}

.page-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: var(--app-frame-inset);
  background-color: #f5f7fa;
  box-sizing: border-box;
  overflow: hidden;
  min-width: 0;
}

.page-aside + .page-main {
  padding-left: 0;
}

@media (prefers-reduced-motion: reduce) {
  .page-aside {
    transition: none;
  }
}
</style>
