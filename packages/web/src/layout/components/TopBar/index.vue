<template>
  <div id="topBar">
    <div class="left">
      <div class="logo" @click="router.push('/')">
        <Logo />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useStore } from 'vuex';
import Logo from './logo.vue';

import './style.scss';
const menus = ref([]);

const router = useRouter();
const route = useRoute();

menus.value = router
  .getRoutes()
  .filter(
    (v) =>
      v.redirect &&
      !['/admin/vgpu'].includes(v.path) && v.name
  )
  .map((item) => ({
    ...item,
    children: item.children.filter((item) => item.meta),
  }));

const store = useStore();

const userInfo = computed(() => {
  return store.state.user;
});

</script>

<style lang="scss"></style>
