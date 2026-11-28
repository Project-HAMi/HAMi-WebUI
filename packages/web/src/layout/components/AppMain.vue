<template>
  <div id="content">
    <div class="content-main-card">
      <div ref="contentBody" class="content-body">
        <router-view />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue';
import { useRoute } from 'vue-router';

const contentBody = ref(null);
const route = useRoute();
watch(() => route.path, () => contentBody.value?.scrollTo(0, 0), { flush: 'post' });
</script>

<style lang="scss" scoped>
#content {
  margin: 0;
  flex: 1;
  min-width: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content-main-card {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #ffffff;
  border-radius: 16px;
  border: 1px solid #e4ebf1;
  box-shadow:
    0 1px 1px 0 rgb(2 5 8 / 2%),
    0 1px 4px 0 rgb(2 5 8 / 6%);
  padding: 16px 0 16px 20px;
  overflow: hidden;
}

.content-body {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  overflow-x: auto;
  padding-right: 12px;
  scrollbar-gutter: stable;
  overscroll-behavior-y: contain;
  scrollbar-width: thin;
  scrollbar-color: #cbd5e1 transparent;
}

.content-body::-webkit-scrollbar {
  width: 6px;
}

.content-body::-webkit-scrollbar-track {
  background-color: transparent;
}

.content-body::-webkit-scrollbar-thumb {
  background-color: #cbd5e1;
  border-radius: 999px;
}
</style>
