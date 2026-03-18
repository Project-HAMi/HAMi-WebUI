<template>
  <div id="content">
    <div class="content-header-row">
      <div class="content-header-spacer" />
      <div class="content-header-actions">
        <LangSelect />
      </div>
    </div>
    <div class="content-main-card">
      <div class="content-body">
        <router-view />
      </div>
    </div>
  </div>
</template>

<script setup>
import LangSelect from '@/components/LangSelect/index.vue';
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

.content-header-row {
  flex-shrink: 0;
  min-height: 0;
  margin-bottom: 4px;
  padding: 0 4px 4px 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  background: transparent;
  border-radius: 0;
  border: none;
  box-shadow: none;
}

.content-header-spacer {
  flex: 1;
}

.content-header-actions {
  flex-shrink: 0;
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
