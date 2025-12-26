<template>
  <div class="text-plus">
    <span :class="{ link: !!to }" @click="handleClick">{{ text }}</span>
    <div v-if="copy && text" class="copy-ico" @click="handleCopy">
      <svg-icon icon="copy" />
    </div>
  </div>
</template>

<script setup>
import { ElMessage } from 'element-plus';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';

const router = useRouter();
const { t } = useI18n();

const props = defineProps({
  text: String,
  copy: Boolean,
  to: null,
});

const handleCopy = () => {
  const textarea = document.createElement('textarea');
  textarea.value = props.text;
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand('copy');
  document.body.removeChild(textarea);

  ElMessage.success(t('common.copySuccess'));
};

const handleClick = () => {
  if (props.to) {
    router.push(props.to);
  }
};
</script>

<style lang="scss">
.text-plus {
  display: inline-flex;
  gap: 10px;
  // width: 100%;
  max-width: 100%;
  
  span {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .link {
    color: var(--el-color-primary);
    cursor: pointer;
    &:hover {
      opacity: 0.8;
    }
  }
  .copy-ico {
    color: #939ea9;
    &:hover {
      cursor: pointer;
      color: #324558;
    }
  }
}
</style>
