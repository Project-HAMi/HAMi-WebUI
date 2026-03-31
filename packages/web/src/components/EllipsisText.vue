<template>
  <el-tooltip
    v-if="tooltipEnabled"
    effect="dark"
    placement="top"
    :content="text"
    :show-after="300"
  >
    <span
      ref="textRef"
      class="ellipsis-text"
      :class="linesClass"
      :style="clampStyle"
    >
      {{ displayText }}
    </span>
  </el-tooltip>

  <span
    v-else
    ref="textRef"
    class="ellipsis-text"
    :class="linesClass"
    :style="clampStyle"
  >
    {{ displayText }}
  </span>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';

const props = defineProps({
  text: {
    type: String,
    default: '',
  },
  mode: {
    type: String,
    default: 'end', // 'end' | 'middle'
  },
  head: {
    type: Number,
    default: 8,
  },
  tail: {
    type: Number,
    default: 6,
  },
  tooltip: {
    type: String,
    default: 'overflow', // 'none' | 'always' | 'overflow'
  },
  lines: {
    type: Number,
    default: 1,
  },
});

const textRef = ref(null);
const isOverflow = ref(false);

const linesClass = computed(() => (props.lines > 1 ? 'is-multiline' : 'is-singleline'));
const clampStyle = computed(() => ({
  '--ellipsis-lines': String(Math.max(1, props.lines)),
}));

const displayText = computed(() => {
  const text = props.text || '';
  if (!text) return '';
  if (props.mode !== 'middle') return text;
  const head = Math.max(0, props.head);
  const tail = Math.max(0, props.tail);
  if (head + tail + 1 >= text.length) return text;
  return `${text.slice(0, head)}…${text.slice(text.length - tail)}`;
});

const tooltipEnabled = computed(() => {
  if (!props.text) return false;
  if (props.tooltip === 'none') return false;
  if (props.tooltip === 'always') return true;
  return isOverflow.value;
});

const checkOverflow = () => {
  const el = textRef.value;
  if (!el) return;
  const hasOverflow = el.scrollHeight > el.clientHeight || el.scrollWidth > el.clientWidth;
  isOverflow.value = hasOverflow;
};

let ro;
onMounted(() => {
  checkOverflow();
  if (typeof ResizeObserver !== 'undefined') {
    ro = new ResizeObserver(() => checkOverflow());
    if (textRef.value) ro.observe(textRef.value);
  }
});

onBeforeUnmount(() => {
  if (ro && textRef.value) ro.unobserve(textRef.value);
  ro = undefined;
});

watch(
  () => [props.text, props.mode, props.head, props.tail, props.lines],
  () => {
    queueMicrotask(checkOverflow);
  },
);
</script>

<style scoped lang="scss">
.ellipsis-text {
  display: inline-block;
  max-width: 100%;
  min-width: 0;
}

.ellipsis-text.is-singleline {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.ellipsis-text.is-multiline {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  overflow: hidden;
  white-space: normal;
  -webkit-line-clamp: var(--ellipsis-lines);
  line-clamp: var(--ellipsis-lines);
}
</style>

