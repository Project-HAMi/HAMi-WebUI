<template>
  <div class="text-plus">
    <span :class="{ link: !!to }" @click="handleClick">
      <EllipsisText
        :text="text"
        :mode="mode"
        :tooltip-class="tooltipClass"
        tooltip="always"
      />
    </span>
  </div>
</template>

<script setup>
import { useRouter } from 'vue-router';
import EllipsisText from './EllipsisText.vue';

const router = useRouter();

const props = defineProps({
  text: String,
  to: null,
  mode: {
    type: String,
    default: 'middle',
  },
  tooltipClass: {
    type: String,
    default: 'vgpu-long-text-tooltip',
  },
});

const handleClick = () => {
  if (props.to) {
    router.push(props.to);
  }
};
</script>

<style lang="scss">
.text-plus {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-width: 0;
  
  > span {
    flex: 1;
    min-width: 0;
    color: #324558;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-flex;
    align-items: center;
  }
  
  .link {
    cursor: pointer;
    &:hover {
      color: var(--el-color-primary);
      opacity: 0.8;
      text-decoration: underline;
      text-underline-offset: 4px;
    }
  }
}
</style>
