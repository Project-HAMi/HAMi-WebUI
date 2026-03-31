<template>
  <el-container class="page">
    <template v-if="!isNoSidebar">
      <el-aside :width="sidebarWidth + 'px'" class="page-aside">
        <Sidebar
          :collapsed="isSidebarCollapsed"
          :menu-width="tMenuWidth"
        />
        <div class="side-nav-compact-button" @click="toggleSidebar">
          <div
            class="side-nav-compact-icon"
            :class="{
              'side-nav-compact-active': isSidebarCollapsed,
              'side-nav-compact-inactive': !isSidebarCollapsed,
            }"
          />
        </div>
      </el-aside>
    </template>
    <el-main class="page-main">
      <AppMain />
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

const tMenuWidth = computed(() => [`${expandedWidth}px`, `${collapsedWidth}px`]);

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
  overflow: visible;
  transition: width 0.3s ease;
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

.side-nav-compact-button {
  position: absolute;
  top: 50%;
  left: calc(100% - 20px);
  transform: translateY(-50%);
  z-index: 10000;

  &:hover {
    left: calc(100% - 12px);
  }

  .side-nav-compact-icon {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    z-index: 100;
    height: 24px;
    width: 24px;
    border-radius: 99px;
    transition: transform 0.5s;

    &::after {
      content: '';
      position: absolute;
      width: 2px;
      height: 16px;
      background: #d5dee7;
    }

    &:hover {
      justify-content: center;
      border-radius: 50%;
      cursor: pointer;
      background-size: 12px 12px;

      &::after {
        display: none;
      }

      &.side-nav-compact-active {
        background: url('@/assets/assets-compact-inactive.svg') no-repeat center center;
        background-color: #fff;
      }

      &.side-nav-compact-inactive {
        background: url('@/assets/assets-compact-active.svg') no-repeat center center;
        background-color: #fff;
      }
    }
  }
}
</style>
