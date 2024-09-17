<template>
  <div id="ResourceName" :class="className">
    {{ name }}
  </div>
</template>

<script setup lang="jsx">
import { defineProps } from 'vue';

const props = defineProps({
  name: String,
  example: Boolean,
  to: String,
});

const className = {
  'overflow-example': props.example,
  link: props.to,
};
</script>

<style lang="scss">
#ResourceName {
  .overflow-example {
    white-space: nowrap;
    /* 防止文本换行 */
    overflow: hidden;
    /* 隐藏溢出的部分 */
    text-overflow: ellipsis;
    /* 显示省略号 */
  }

  p {
    margin-bottom: 0;
  }
}
</style>
