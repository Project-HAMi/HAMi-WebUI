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

<style lang="scss">
.home-block {
  border-radius: 8px;
  background: #fff;
  margin-bottom: 15px;
  box-shadow: 0px 1px 1px 0px rgba(2, 5, 8, 0.02),
    0px 1px 4px 0px rgba(2, 5, 8, 0.06);
  padding: 20px;
  &-header {
    display: flex;
    justify-content: space-between;
    .title {
      color: #1d2b3a;
      font-family: 'PingFang SC';
      font-size: 14px;
      font-style: normal;
      font-weight: 500;
      line-height: 20px; /* 142.857% */
      margin-bottom: 20px;
    }
  }
}
</style>
