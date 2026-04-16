<template>
  <div class="home-block">
    <div class="home-block-header" v-if="title">
      <div class="title">{{ title }}</div>
      <div class="extra">
        <slot name="extra" />
      </div>
    </div>
    <div class="home-block-content"><slot /></div>
  </div>
</template>

<script setup>
defineProps(['title']);
</script>

<style scoped lang="scss">
.home-block {
  border-radius: 12px;
  background: #fff;
  border: 1px solid #e4ebf1;
  padding: 20px;
  &-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 15px;
    .title {
      color: #1d2b3a;
      font-family: 'PingFang SC';
      font-size: 16px;
      font-style: normal;
      font-weight: 500;
      line-height: 20px; /* 142.857% */
      margin-bottom: 0;
    }
  }
}
</style>
