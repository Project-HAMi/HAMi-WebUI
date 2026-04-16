<template>
  <el-page-header class="layout-page-header" @back="goBack">
    <template #content>
      <div class="layout-header-title">
        <slot name="title">
          <h3 class="layout-title">{{ title }}：{{ name }}</h3>
        </slot>
        <div v-if="status" class="layout-header-title-run-state">
          <slot name="icon">
            <svg-icon v-if="statusIcon" :icon="statusIcon" style="font-size: 16px" />
          </slot>
          <span>{{ status }}</span>
          <slot name="titleSuffix"></slot>
        </div>
      </div>
    </template>
    <template #extra>
      <slot name="actions"></slot>
    </template>
  </el-page-header>
</template>

<script setup>
import { useRouter } from 'vue-router';

const router = useRouter();

const goBack = () => {
  router.go(-1);
};

defineProps({
  title: {
    type: String,
    default: '',
  },
  name: {
    type: String,
    default: '',
  },
  status: {
    type: String,
    default: '',
  },
  statusIcon: {
    type: String,
    default: '',
  },
});
</script>

<style scoped lang="scss">
.layout-page-header {
  padding: 0 0 8px 0;
  margin-bottom: 12px;
}

.layout-header-title {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #1d2b3a;
  font-size: 18px;
  font-weight: 500;
  line-height: 36px;
}

.layout-title {
  margin: 0;
  font-size: 21px;
}

.layout-header-title-run-state {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #324558;
  font-size: 14px;
  line-height: 24px;
}

:deep(.el-page-header__back) {
  color: #999;
  &:hover {
    color: var(--el-color-primary);
  }
}
</style>
