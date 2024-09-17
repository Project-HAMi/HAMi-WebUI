<template>
  <div>
    <el-pagination
      class="pagination"
      v-model:page-size="currentParams.pageRequest.pageSize"
      v-model:currentPage="currentParams.pageRequest.pageNo"
      background
      layout="total, ->, sizes, jumper, prev, next"
      :page-sizes="[5, 10, 20, 50, 100, 500]"
      :total="total"
      :pager-count="3"
    />
  </div>
</template>

<script setup lang="jsx">
import { defineProps, inject } from 'vue';

const props = defineProps({
  total: {
    type: Number,
    default: 0,
  },
});

const currentParams = inject('currentParams');
</script>
