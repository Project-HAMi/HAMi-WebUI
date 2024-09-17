<template>
  <ElTransfer
    v-bind="$attrs"
    v-model="selectVal"
    :data="selectOptions"
    :props="{ label: props.labelKey, key: props.valueKey }"
  />
</template>

<script setup>
import {
  defineProps,
  defineEmits,
  ref,
  reactive,
  computed,
  watch,
  onMounted,
  inject,
} from 'vue';
import { isEqual, isPlainObject, debounce, isFunction, isArray } from 'lodash';
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
  api: Object,
  disabled: Boolean,
});

const emits = defineEmits(['update:modelValue', 'onChangeSelect']);

const $selectData = inject('$selectData');

const loading = ref(false);

const selectOptions = ref([]);

const updateSelectData = (val) => {
  if (!val) return;
  const { name, valueKey } = props;

  let selectData = {};

  selectData = selectOptions.value.find((item) => item[valueKey] === val);

  //如果接到了$selectData，给顶级组件保存当前值对应得数据源
  if ($selectData) {
    $selectData[name] = selectData;
  }
};

const selectVal = computed({
  get() {
    updateSelectData(props.modelValue);
    return props.modelValue;
  },
  set(val) {
    emits('update:modelValue', val);
  },
});

const fetchData = debounce(async () => {
  const { url, method, params, data } = props.api;
  loading.value = true;

  const { list } = await request({
    url,
    method,
    params,
    data,
  });

  selectOptions.value = props.formatterOptions
    ? props.formatterOptions(list)
    : list.map((item) => {
        if (isPlainObject(item)) {
          return item;
        }
        return { label: item, value: item };
      });
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
  () => props.options,
  (newVal) => {
    if (props.mode === 'static') {
      selectOptions.value = newVal;
    }
  },
);

watch(
  () => props.api,
  (newVal, oldVal) => {
    //bug：这里发生只api内存地址变化，实际api无变化也会触发监听。暂时使用深层对比解决
    if (!isEqual(newVal, oldVal)) {
      fetchData();
    }
  },
);
</script>
