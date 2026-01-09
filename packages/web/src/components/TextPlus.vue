<template>
  <div class="text-plus">
    <span :class="{ link: !!to }" @click="handleClick">{{ text }}</span>
    <div v-if="copy && text" class="copy-ico" @click="handleCopy">
      <svg-icon icon="copy" />
    </div>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router';
import { copy } from '@/utils';

const router = useRouter();

const props = defineProps({
  text: String,
  copy: Boolean,
  to: null,
});

const handleCopy = () => {
  copy(props.text);
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
