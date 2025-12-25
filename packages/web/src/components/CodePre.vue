<template>
  <div class="code-pre">
    <div class="copy-ico" @click="handleCopy">
      <svg-icon icon="copy" />
    </div>
    <pre>{{ value }}</pre>
  </div>
</template>

<script setup>
import { copy } from '@/utils';

const props = defineProps({
  value: String,
});

const handleCopy = () => {
  copy(props.value);
};
</script>

<style scoped lang="scss">
.code-pre {
  position: relative;
  .copy-ico {
    position: absolute;
    top: 10px;
    right: 10px;
    color: #fff;
    font-size: 20px;
    &:hover {
      cursor: pointer;
      color: var(--el-color-primary);
    }
  }
}
</style>
