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
      <div class="detail-page-skeleton__summary" aria-hidden="true">
        <t-skeleton
          animation="gradient"
          :row-col="[
            { width: '32%', height: '20px' },
            { width: '100%', height: '20px' },
            { width: '88%', height: '20px' },
            { width: '72%', height: '20px' },
          ]"
        />
      </div>
      <div class="detail-page-skeleton__cards" aria-hidden="true">
        <div v-for="index in 4" :key="index" class="detail-page-skeleton__card">
          <t-skeleton
            animation="gradient"
            :row-col="[
              { width: '54%', height: '24px' },
              { width: '82%', height: '16px' },
            ]"
          />
        </div>
      </div>
      <div class="detail-page-skeleton__charts" aria-hidden="true">
        <div v-for="index in 2" :key="index" class="detail-page-skeleton__chart">
          <t-skeleton
            animation="gradient"
            :row-col="[
              { width: '44%', height: '20px' },
              { width: '100%', height: '180px' },
            ]"
          />
        </div>
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

.detail-page-skeleton {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-page-skeleton__summary,
.detail-page-skeleton__card,
.detail-page-skeleton__chart {
  padding: 20px;
  border: 1px solid #e4ebf1;
  border-radius: 12px;
  background: #fff;
}

.detail-page-skeleton__summary {
  min-height: 150px;
}

.detail-page-skeleton__cards,
.detail-page-skeleton__charts {
  display: grid;
  gap: 16px;
}

.detail-page-skeleton__cards {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.detail-page-skeleton__card {
  min-height: 112px;
}

.detail-page-skeleton__charts {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.detail-page-skeleton__chart {
  min-height: 260px;
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

@media (max-width: 1200px) {
  .detail-page-skeleton__cards,
  .detail-page-skeleton__charts {
    grid-template-columns: 1fr;
  }
}
</style>
