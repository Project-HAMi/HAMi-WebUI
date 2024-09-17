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
  background: #fff;
  padding: 15px 10px;
  margin-bottom: 20px;
  border-radius: 10px;
  box-shadow: 0px 0px 6px rgba(0, 0, 0, 0.12);
  .el-page-header__back {
    color: #999;
    &:hover {
      color: var(--el-color-primary);
    }
  }
}
</style>
