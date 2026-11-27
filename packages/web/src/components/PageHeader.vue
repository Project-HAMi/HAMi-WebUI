<template>
  <header class="layout-page-header">
    <button
      type="button"
      class="layout-page-header__back"
      @click="goBack"
    >
      <ArrowLeftIcon class="layout-page-header__back-icon" />
      <span>{{ $t('common.back') }}</span>
    </button>
    <span class="layout-page-header__divider" aria-hidden="true" />
    <div class="layout-header-title">
      <slot name="title">
        <h3 class="layout-title">{{ title }}：{{ name }}</h3>
      </slot>
      <div v-if="status" class="layout-header-title-run-state">
        <slot name="status">
          <slot name="icon">
            <svg-icon v-if="statusIcon" :icon="statusIcon" style="font-size: 16px" />
          </slot>
          <span class="layout-header-title-run-state-label">{{ status }}</span>
          <slot name="titleSuffix"></slot>
        </slot>
      </div>
    </div>
    <div v-if="$slots.actions" class="layout-page-header__extra">
      <slot name="actions"></slot>
    </div>
  </header>
</template>

<script setup>
import { ArrowLeftIcon } from 'tdesign-icons-vue-next';
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
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding: 0 0 8px 0;
  margin-bottom: 12px;
}

.layout-page-header__back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  border: 0;
  background: transparent;
  color: #6b7785;
  font: inherit;
  font-size: 14px;
  line-height: 20px;
  cursor: pointer;

  &:hover {
    color: var(--td-brand-color, #2563eb);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color, #2563eb);
    outline-offset: 2px;
    border-radius: 4px;
  }
}

.layout-page-header__back-icon {
  font-size: 16px;
}

.layout-page-header__divider {
  width: 1px;
  height: 16px;
  background: #dcdfe6;
}

.layout-header-title {
  display: flex;
  flex: 1;
  min-width: 0;
  align-items: baseline;
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

  &-label {
    align-self: baseline;
  }
}

.layout-page-header__extra {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
