<template>
  <el-select
    v-model="selectVal"
    :placeholder="placeholder"
    @change="selectChange"
    :disabled="disabled"
    :multiple="multiple"
    :size="size"
    :style="style"
    :clearable="clearable"
    filterable
  >
    <template #header v-if="multiple">
      <el-checkbox
        v-model="checkAll"
        :indeterminate="indeterminate"
        @change="handleCheckAll"
      >
        全选
      </el-checkbox>
    </template>
    <el-option
      v-for="item in selectOptions"
      :key="item[valueKey]"
      :label="formatter ? formatter(item[labelKey]) : item[labelKey]"
      :value="item[valueKey]"
      :disabled="item.disabled"
    >
      <div class="selectPlus-option">
        <span class="optionAvatar">
          <svg-icon
            v-if="optionAvatarIcon"
            :icon="
              isFunction(optionAvatarIcon)
                ? optionAvatarIcon(item)
                : optionAvatarIcon
            "
          />
        </span>

        <span>
          {{ formatter ? formatter(item[labelKey]) : item[labelKey] }}</span
        >
      </div>
    </el-option>
  </el-select>
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
  placeholder: {
    type: String,
    default: '请选择',
  },
  clearable: {
    type: Boolean,
    default: true,
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  multiple: {
    type: Boolean,
    default: false,
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
  autoSelectedFirst: {
    type: Boolean,
    default: false,
  },
  autoSelectedLast: {
    type: Boolean,
    default: false,
  },
  api: Object,
  name: String,
  size: {
    type: String,
    default: 'default',
  },
  style: null,
  filterKey: { default: 'filter', type: String },
  formatter: Function,
  sort: Boolean,
  optionAvatarIcon: String,
  formatterOptions: Function,
  effect: null,
  dataPath: { default: 'list', type: String },
  defaultSelectedAll: Boolean,
});

const emits = defineEmits(['update:modelValue', 'onChangeSelect']);

const selectOptions = ref([]);

const loading = ref(false);

const checkAll = ref(false);
const indeterminate = ref(false);

const stateParams = reactive({
  // pageNum: 1,
  // pageSize: 10,
  // filters: {},
  // orderBys: {},
});

const $selectData = inject('$selectData');

const updateSelectData = (val) => {
  if (!val) return;
  const { name, valueKey, multiple } = props;

  let selectData = {};

  if (multiple) {
    //多选就过滤出vals对应的源数据
    selectData = selectOptions.value.filter((item) => {
      return val.includes(item[valueKey]);
    });
  } else {
    //单选找到单项对应的源数据
    selectData = selectOptions.value.find((item) => item[valueKey] === val);
  }

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
    // const { multiple, defaultSelectedAll, valueKey } = props;
    // if (multiple && defaultSelectedAll && val.length === 0) {
    //   return emits(
    //     'update:modelValue',
    //     selectOptions.value.map((item) => item[valueKey]),
    //   );
    // }

    emits('update:modelValue', val);
  },
});

const fetchData = debounce(async () => {
  const { url, method, params, data } = props.api;
  loading.value = true;

  const { [props.dataPath]: list } = await request({
    url,
    method,
    params: { ...params, ...stateParams },
    data: { ...data, ...stateParams },
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

const handleCheckAll = (val) => {
  indeterminate.value = false;
  if (val) {
    selectVal.value = selectOptions.value.map((_) => _[props.valueKey]);
  } else {
    selectVal.value = [];
  }
};

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

watch(selectVal, (val) => {
  if (!isArray(val)) return (checkAll.value = false);

  if (val.length === 0) {
    checkAll.value = false;
    indeterminate.value = false;
  } else if (val.length === selectOptions.value.length) {
    checkAll.value = true;
    indeterminate.value = false;
  } else {
    indeterminate.value = true;
  }
});

watch(selectOptions, (newVal) => {
  const {
    autoSelectedFirst,
    autoSelectedLast,
    modelValue,
    valueKey,
    multiple,
    sort,
    defaultSelectedAll,
  } = props;
  // 自动选中第一项
  if (autoSelectedFirst && newVal.length && !modelValue?.length) {
    emits(
      'update:modelValue',
      multiple ? [newVal[0][valueKey]] : newVal[0][valueKey],
    );
    selectChange(multiple ? [newVal[0][valueKey]] : newVal[0][valueKey]);
  }
  if (autoSelectedLast && newVal.length && !modelValue?.length) {
    emits(
      'update:modelValue',
      multiple ? [newVal[newVal.length - 1][valueKey]] : newVal[newVal.length - 1][valueKey],
    );
    selectChange(multiple ? [newVal[newVal.length - 1][valueKey]] : newVal[newVal.length - 1][valueKey]);
  }

  if (sort) {
    selectOptions.value = selectOptions.value.sort((a, b) => a.value - b.value);
  }

  if (multiple && defaultSelectedAll) {
    selectVal.value = newVal.map((item) => item[valueKey]);
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
  () => props.effect,
  (newVal) => {
    fetchData();
  },
);

const selectChange = (val) => {
  emits('onChangeSelect', updateSelectData(val));
};
</script>

<style lang="scss" scoped>
.selectPlus-option {
  display: flex;

  .optionAvatar {
    font-size: 20px;
    margin-right: 10px;
  }
}
</style>
