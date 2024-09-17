<template>
  <div id="content" :style="{ paddingLeft: isHome ? 0 : '20px' }">
    <!-- <transition name="fade-transform" mode="out-in"> -->
    <!-- <keep-alive> -->
    <router-view />
    <!-- </keep-alive> -->
    <!-- </transition> -->
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useRoute } from 'vue-router';

const route = useRoute();

const isHome = computed(() => route.fullPath === '/admin/home');
</script>

<style lang="scss" scoped>
#content {
  margin: 0;
  flex: 1;
  overflow: auto;

  height: 100%;
}
</style>
