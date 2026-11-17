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

import amd from '@/assets/vendor/amd.png';
import mthreads from '@/assets/vendor/mthreads.png';
import iluvatar from '@/assets/vendor/iluvatar.png';
import enflame from '@/assets/vendor/enflame.png';
import kunlunxin from '@/assets/vendor/kunlunxin.png';
import aws from '@/icons/svg/gpu-aws.svg?url';
import biren from '@/assets/vendor/biren.png';
import vastai from '@/assets/vendor/vastai.png';
import tecorigin from '@/assets/vendor/tecorigin.png';

const marks = new Map([
  ['NVIDIA', nvidia],
  ['Ascend', ascend],
  ['Metax', metax],
  ['MLU', cambricon],
  ['Hygon', hygon],
  ['AMD', amd],
  ['Mthreads', mthreads],
  ['Iluvatar', iluvatar],
  ['Enflame', enflame],
  ['Kunlunxin', kunlunxin],
  ['AWS', aws],
  ['Biren', biren],
  ['Vastai', vastai],
  ['Tecorigin', tecorigin],
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
