<template>
  <el-cascader
    v-model="selectVal"
    @change="selectChange"
    :props="{
      label: labelKey,
      value: valueKey,
      children: childrenKey,
      multiple,
    }"
    :options="selectOptions"
    v-bind="$attrs"
    :key="effect"
  />
</template>

<script setup>
import {
  defineProps,
  defineEmits,
  ref,
  computed,
  watch,
  inject,
  onMounted,
  watchEffect,
} from 'vue';
import { isString, isEqual, debounce } from 'lodash';
import request from '@/utils/request';

const props = defineProps({
  modelValue: {},
  options: {
    type: Array,
    default: () => [],
  },
  mode: {
    type: String,
    default: 'static',
  },
  labelKey: {
    type: String,
    default: 'label',
  },
  valueKey: {
    type: String,
    default: 'value',
  },
  childrenKey: {
    type: String,
    default: 'children',
  },
  api: Object,
  name: String,
  formatterOptions: Function,
  multiple: Boolean,
  effect: null,
});

const emits = defineEmits(['update:modelValue', 'onChangeSelect']);

const selectVal = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emits('update:modelValue', val);
  },
});

const selectOptions = ref([]);

const loading = ref(false);

const $selectData = inject('$selectData');

const fetchData = debounce(async () => {
  const { url, method, data = {} } = props.api;

  loading.value = true;

  const { list } = await request({
    url,
    method,
    data,
  });

  selectOptions.value = props.formatterOptions
    ? props.formatterOptions(list)
    : list;

  loading.value = false;
}, 300);

onMounted(() => {
  const { mode, options } = props;
  if (mode === 'static') {
    selectOptions.value = options;
  }
  if (mode === 'remote') {
    fetchData();
  }
});

watch(
  () => props.api,
  (newVal, oldVal) => {
    //bug：这里发生只api内存地址变化，实际api无变化也会触发监听。暂时使用深层对比解决
    if (!isEqual(newVal, oldVal)) {
      fetchData();
    }
  },
);

watch(
  () => props.effect,
  (newVal) => {
    fetchData();
  },
);

const selectChange = (val) => {
  const { name, valueKey } = props;

  const item = selectOptions.value.find((item) => item[valueKey] === val);

  $selectData[name] = item;
};
</script>

<style lang="scss" scoped></style>
