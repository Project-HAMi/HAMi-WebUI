<script setup>
import { ref, defineModel } from 'vue';

const time = defineModel();

const timeOptions = ref([
  {
    label: '近7天',
    value: 7,
  },
  {
    label: '近5天',
    value: 5,
  },
  {
    label: '近3天',
    value: 3,
  },
  {
    label: '24小时内',
    value: 1,
  },
  {
    label: '12小时内',
    value: 0.5,
  },
  {
    label: '1小时内',
    value: 0.5 / 12,
  },
]);
</script>

<template>
  <el-select v-model="time" style="width: 150px">
    <el-option
      v-for="{ label, value } in timeOptions"
      :label="label"
      :value="value"
    ></el-option>
  </el-select>
</template>

<style scoped lang="scss"></style>
