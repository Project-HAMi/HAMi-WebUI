<template>
  <img v-if="mark" class="vendor-mark" :src="mark" alt="" :width="size" :height="size">
  <slot v-else />
</template>

<script>
import nvidia from '@/icons/svg/gpu-nvidia.svg?url';
import ascend from '@/icons/svg/gpu-ascend.svg?url';
import metax from '@/icons/svg/gpu-metax.svg?url';
import cambricon from '@/assets/vendor/cambricon.png';
import hygon from '@/assets/vendor/hygon.png';

const marks = new Map([
  ['NVIDIA', nvidia],
  ['Ascend', ascend],
  ['Metax', metax],
  ['MLU', cambricon],
  ['Hygon', hygon],
]);
</script>

<script setup>
import { computed } from 'vue';
import { gpuVendor } from '../metrics/vendor-colors.mjs';

const props = defineProps({
  vendor: String,
  size: { type: Number, default: 16 },
});
const mark = computed(() => marks.get(gpuVendor(props.vendor)));
</script>

<style scoped>
.vendor-mark { flex: none; object-fit: contain; }
</style>
