<template>
  <div class="code-pre">
    <div class="copy-ico" @click="handleCopy">
      <svg-icon icon="copy" />
    </div>
    <pre>{{ value }}</pre>
  </div>
</template>

<script setup>
import { ElMessage } from 'element-plus';

const props = defineProps({
  value: String,
});

const handleCopy = () => {
  const textarea = document.createElement('textarea');
  textarea.value = props.value;
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand('copy');
  document.body.removeChild(textarea);

  ElMessage.success('复制成功');
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
