<template>
  <el-menu
    :default-active="this.activePath"
    id="sidebar"
    :class="$store.state.layout.sidebarCollapse && 'sidebarCollapse'"
    :collapse="$store.state.layout.sidebarCollapse"
    router
    v-if="this.activePath !== '/dashboard'"
  >
    <el-menu-item
      class="collapseBtn"
      @click="$store.commit('changeSidebarCollapse')"
      :style="!$store.state.layout.sidebarCollapse && 'width: 250px'"
    >
      <div>
        <el-icon>
          <Expand />
        </el-icon>
        <span v-if="!$store.state.layout.sidebarCollapse">{{
          $route.matched[0].meta.title
        }}</span>
      </div>
    </el-menu-item>

    <template v-for="item in routes">
      <el-sub-menu :key="item.path" :index="item.path" v-if="item.children">
        <template #title>
          <el-icon><svg-icon :icon="item.meta.icon || 'instance'" /> </el-icon>
          <span>{{ item.meta.title }}</span>
        </template>

        <template
          v-for="(v, index) in item.children.filter((child) => child.meta)"
          :key="index"
        >
          <el-menu-item
            v-if="isURL(v.path)"
            @click="handleGoLink(v.path)"
            class="threeRoute"
          >
            {{ v.meta?.title }}
          </el-menu-item>

          <el-menu-item v-else class="threeRoute" :key="v.path" :index="v.path">
            <el-icon><svg-icon :icon="v.meta.icon || 'instance'" /> </el-icon>
            <template #title>{{ v.meta.title }}</template>
          </el-menu-item>
        </template>
      </el-sub-menu>

      <el-menu-item :index="item.path" :key="item.name" v-else>
        <el-icon><svg-icon :icon="item.meta.icon || 'instance'" /> </el-icon>
        <template #title>{{ item.meta.title }}</template>
      </el-menu-item>
    </template>
  </el-menu>
</template>

<script>
import { Expand } from '@element-plus/icons-vue';
export default {
  components: { Expand },
  props: {},
  data() {
    return {
      activePath: '',
      routes: this.$route.matched[0].children.filter((item) => item.meta),
    };
  },
  mounted() {
    this.activePath = this.$route.path;
  },
  watch: {
    $route: {
      handler: function (val, oldVal) {
        this.activePath = val.path;
        this.routes = val.matched[0].children.filter((item) => item.meta);
      },
      // 深度观察监听
      deep: true,
    },
  },
  methods: {
    isURL(str) {
      const urlRegex = str.startsWith('https://') || str.startsWith('http://');
      return urlRegex;
    },
    handleGoLink(url) {
      window.open(url);
    },
  },
};
</script>
<style lang="scss" scoped>
#sidebar {
  overflow: auto;

  .twoMenu {
    font-size: 13px;
  }

  .collapseBtn {
    display: flex;
    justify-content: center;
    flex-direction: column;
    font-size: 16px;
    font-weight: bold;
  }

  .threeRoute {
    padding-left: 65px;
  }
}

.sidebarCollapse {
  width: 64px !important;
}

.el-menu-vertical-demo:not(.el-menu--collapse) {
  width: 250px;
  height: 100%;
}
</style>
