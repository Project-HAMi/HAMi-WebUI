<template>
  <section
    class="detail-page-state"
    :aria-busy="status === REQUEST_STATUS.LOADING ? 'true' : 'false'"
    :data-detail-state="status"
  >
    <div
      v-if="status === REQUEST_STATUS.LOADING"
      class="detail-page-skeleton"
      data-testid="detail-page-skeleton"
    >
      <span class="detail-page-state__sr-only" role="status">
        {{ $t('common.loading') }}
      </span>
      <!-- Each page owns its loading geometry through the same layout classes as its content. -->
      <div aria-hidden="true" inert>
        <slot name="loading" />
      </div>
    </div>

    <div
      v-else-if="status === REQUEST_STATUS.MISSING"
      class="detail-page-feedback"
      data-testid="detail-page-missing"
      role="status"
    >
      <el-empty :description="$t('common.resourceNotFound')" />
    </div>

    <div
      v-else-if="status !== REQUEST_STATUS.READY"
      class="detail-page-feedback"
      data-testid="detail-page-error"
      role="alert"
    >
      <el-empty :description="$t('common.loadFailed')">
        <el-button type="primary" data-testid="detail-page-retry" @click="$emit('retry')">
          {{ $t('common.retry') }}
        </el-button>
      </el-empty>
    </div>

    <slot v-else />
  </section>
</template>

<script setup>
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';

defineProps({
  status: {
    type: String,
    required: true,
  },
});

defineEmits(['retry']);
</script>

<style scoped lang="scss">
.detail-page-state {
  min-height: 320px;
}

.detail-page-feedback {
  display: flex;
  min-height: 420px;
  align-items: center;
  justify-content: center;
  padding: 24px;
  border: 1px solid #e4ebf1;
  border-radius: 12px;
  background: #fff;
}

.detail-page-state__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

</style>
