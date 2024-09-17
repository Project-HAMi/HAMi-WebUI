<template>
  <TopBar />
  <div class="page">
    <sidebar v-if="!isNoSidebar()" />
    <app-main />
  </div>
</template>

<script>
import '@tabler/core/dist/css/tabler.min.css';
import { AppMain, Sidebar, TopBar } from './components';

export default {
  name: 'Layout',
  components: {
    AppMain,
    Sidebar,
    TopBar,
  },
  created() {
    this.isNoSidebar();
  },
  methods: {
    isNoSidebar() {
      const noSidebarPaths = ['/admin/home', '/admin/message-center', "/admin/about-system", "/admin/settings/config-map"];
      return noSidebarPaths.includes(this.$route.fullPath);
    }
  },
};
</script>

<style lang="scss" scoped>
@import '~@/styles/mixin.scss';
@import '~@/styles/variables.scss';

.page {
  display: flex;
  flex-direction: row;
  padding: 20px;
  padding-top: 76px;
  height: calc(100vh);
}

.app-wrapper {
  @include clearfix;
  position: relative;
  height: 100%;
  width: 100%;

  /* &.mobile.openSidebar {
    position: fixed;
    top: 0;
  } */
}

.drawer-bg {
  background: #000;
  opacity: 0.3;
  width: 100%;
  top: 0;
  height: 100%;
  position: absolute;
  z-index: 999;
}

.fixed-header {
  position: fixed;
  top: 0;
  right: 0;
  z-index: 9;
  width: calc(100% - #{$sideBarWidth});
  transition: width 0.28s;
}

.hideSidebar .fixed-header {
  width: calc(100% - 54px);
}

.mobile .fixed-header {
  width: 100%;
}
</style>
