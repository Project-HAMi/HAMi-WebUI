<template>
  <el-container class="page">
    <template v-if="!isNoSidebar">
      <el-aside :width="sidebarWidth + 'px'" class="page-aside">
        <Sidebar
          :collapsed="isSidebarCollapsed"
          @toggle="toggleSidebar"
        />
      </el-aside>
    </template>
    <el-main class="page-main">
      <AppMain :show-language-switch="isNoSidebar" />
    </el-main>
  </el-container>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import '@tabler/core/dist/css/tabler.min.css';
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
  display: flex;
  flex-direction: row;
  padding: 0;
  height: 100vh;
  box-sizing: border-box;
  overflow: hidden;
}

.page-aside {
  position: relative;
  flex-shrink: 0;
  padding: 0;
  background-color: transparent;
  border: none;
  overflow: hidden;
  transition: width 180ms cubic-bezier(0.2, 0, 0, 1);
}

.page-main {
  display: flex;
  flex-direction: column;
  padding: 8px !important;
  background-color: #f5f7fa;
  box-sizing: border-box;
  overflow: hidden;
  min-width: 0;
}

@media (prefers-reduced-motion: reduce) {
  .page-aside {
    transition: none;
  }
}
</style>
