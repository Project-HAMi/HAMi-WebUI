<template>
  <div class="info-preview">
    <el-descriptions :title="title" :column="column" style="padding: 20px">
      <el-descriptions-item
        :label="label"
        :key="value"
        v-for="{ label, value, render } in columns"
      >
        <span v-if="render">
          <component
            v-if="isVNode(render(data))"
            :is="render(data)"
          ></component>
          <span v-else>{{ render(data) }}</span>
        </span>
        <span v-else>
          {{ data[value] }}
        </span>
      </el-descriptions-item>
    </el-descriptions>
    <button-group :items="actions" :record="data" />
  </div>
</template>

<script setup lang="jsx">
import { defineProps, isVNode } from 'vue';

defineProps({
  data: Object,
  column: {
    type: Number,
    default() {
      return 2;
    },
  },
  columns: Array,
  title: String,
  actions: Array,
});
</script>

<style lang="scss">
.info-preview {
  .el-descriptions__label {
    min-width: 150px;
    display: inline-block;
  }
}
</style>
