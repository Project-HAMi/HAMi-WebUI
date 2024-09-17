<template>
  <div class="navbar">
    <el-menu mode="horizontal" :default-active="$route.fullPath" :router="true">
      <el-menu-item v-for="item in items" :key="item.path" :index="item.path"><i :class="item.meta.icon" />{{ item.meta.title }}</el-menu-item>
    </el-menu>
  </div>
</template>

<script>
import { mapGetters } from 'vuex'
export default {
  name: 'Navbar',
  props: {
    navbarKey: {
      type: String,
      default: 'Index'
    },
    navbarSubKey: {
      type: String,
      default: 'Info'
    }
  },
  data() {
    return {
      items: []
    }
  },
  computed: {
    ...mapGetters([
      'permission_routes'
    ])
  },
  created() {
  }
}
</script>

<style scoped lang="scss">
.navbar {
  margin-bottom: 3px;
}
::v-deep .el-menu--horizontal > .el-menu-item {
  height: 40px;
  line-height: 40px;
}
</style>
