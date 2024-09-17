<template>
  <div :class="['ListHeader', isExpand && 'isExpand']" v-bind="$attrs">
    <div class="ico">
      <svg-icon :icon="icon || route.meta.icon" />
    </div>

    <div class="info">
      <h3 class="title">{{ title || route.meta.title }}</h3>
      <p v-if="!isExpand" class="desc">
        <span v-if="description"> {{ description || route.meta.title }}</span>
        <slot v-else name="description" />
      </p>
    </div>

    <div class="actions">
      <slot name="actions" />
    </div>

    <svg-icon icon="block-bg" class="list-head-bg" />
    <div class="list-head-expand" @click="isExpand = !isExpand">
      <svg-icon :icon="`list-head-${isExpand ? 'expand' : 'narrow'}`" />
    </div>
  </div>
</template>

<script setup>
import { defineProps, ref } from 'vue';
import { useRoute } from 'vue-router';

const props = defineProps({
  title: String,
  description: String,
  icon: String,
});

const isExpand = ref(false);

const route = useRoute();
</script>

<style scoped lang="scss">
.ListHeader {
  height: 88px;
  .ico {
    // min-width: 72px;
    height: 100%;
    display: flex;
    justify-content: center;
    align-items: center;
    font-size: 40px;
    padding: 28px 24px;
  }
  .info {
    display: flex;
    flex-direction: column;
    justify-content: center;
    margin-right: auto;
    .title {
      color: #1d2b3a;

      font-family: 'PingFang SC';
      font-size: 16px;
      font-style: normal;
      font-weight: 500;
      line-height: 24px; /* 150% */
    }
    .desc {
      color: #939ea9;

      font-family: 'PingFang SC';
      font-size: 12px;
      font-style: normal;
      font-weight: 400;
      line-height: 20px; /* 166.667% */
      margin-top: 4px;
    }
  }
  .actions {
    // line-height: 72px;
    padding-right: 15px;
    z-index: 10;
    display: flex;
    align-items: center;

    gap: 10px;
    .el-button {
      margin-left: 0;
    }
  }

  .list-head-bg {
    width: 336px;
    height: 119px;
    position: absolute;
    right: 0;
    top: -10px;
  }

  .list-head-expand {
    position: absolute;
    right: 0;
    top: 0;
    z-index: 20;
    display: inline-flex;
    padding: 4px 6px;
    flex-direction: column;
    justify-content: center;
    align-items: center;
    border-radius: 0px 8px;
    background: #fff;

    box-shadow: 0px 1px 1px 0px rgba(2, 5, 8, 0.02),
      0px 1px 4px 0px rgba(2, 5, 8, 0.06);
    cursor: pointer;
  }

  position: relative;
  overflow: hidden;
  display: flex;
  margin-bottom: 12px;
  border-radius: 3px;
  box-shadow: rgba(26, 30, 34, 0.08) 0px 1px 3px;
  border-radius: 8px;
  border: 1px solid #fff;
  background: linear-gradient(90deg, #f5f7fa 0%, #e2eefe 100%);

  box-shadow: 0px 1px 1px 0px rgba(2, 5, 8, 0.02),
    0px 1px 4px 0px rgba(2, 5, 8, 0.06);
}

p,
h3 {
  margin-bottom: 0;
}
p {
  font-size: 12px;
  padding-right: 5px;
}

.isExpand {
  height: 48px;
  .ico {
    font-size: 24px;
    padding: 12px 16px;
  }
}
</style>
