<template>
  <el-page-header id="BackHeader" @back="goBack">
    <template #content>
      <slot />
    </template>
    <template #extra>
      <slot name="extra" />
    </template>
  </el-page-header>
</template>

<script setup lang="jsx">
import { useRouter } from 'vue-router';

const router = useRouter();

const goBack = () => {
  router.go(-1);
};
</script>

<style lang="scss">
#BackHeader {
  padding: 0 0 8px 0;
  margin-bottom: 12px;

  .el-page-header__back {
    color: #999;
    &:hover {
      color: var(--el-color-primary);
    }
  }

  .el-page-header__content {
    font-size: 18px;
    font-weight: 500;
    color: #1d2b3a;
  }
}
</style>
