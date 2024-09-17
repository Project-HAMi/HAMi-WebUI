<template>
  <el-radio-group
    v-model="selectVal"
    :placeholder="placeholder"
    :disabled="disabled"
    @change="selectChange"
    :class="{ 'radio-plus': true, renderLabelIcon }"
  >
    <template v-if="optionType === 'circle'">
      <el-radio
        v-for="(item, index) in selectOptions"
        :key="item[valueKey]"
        :label="item[valueKey]"
        size="large"
        >{{ renderLabel ? renderLabel(item, index) : item[labelKey] }}</el-radio
      >
    </template>

    <el-space v-if="optionType === 'button'" wrap>
      <el-radio-button
        v-for="(item, index) in selectOptions"
        :key="item[valueKey]"
        :label="item[valueKey]"
        size="large"
        class="radio-button-item"
        :disabled="item.disabled"
        ><div>
          <div class="radio-button-item-icon" v-if="renderLabelIcon">
            <svg-icon :icon="renderLabelIcon(item, index)" />
          </div>
          <div>
            {{ renderLabel ? renderLabel(item, index) : item[labelKey] }}
          </div>
        </div>
      </el-radio-button>
    </el-space>

    <!-- <el-space v-if="optionType === 'block'" wrap>
      <el-radio-button
        v-for="(item, index) in selectOptions"
        :key="item[valueKey]"
        :label="item[valueKey]"
        size="large"
        class="radio-button-item"
        ><div>
          <div class="radio-button-item-icon" v-if="renderLabelIcon">
            <svg-icon :icon="renderLabelIcon(item, index)" />
          </div>
          <div>
            {{ renderLabel ? renderLabel(item, index) : item[labelKey] }}
          </div>
        </div>
      </el-radio-button>
    </el-space> -->
  </el-radio-group>
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
} from 'vue';
import { isString, isEqual, debounce } from 'lodash';
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
  disabled: {
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
  api: Object,
  name: String,
  optionType: {
    type: String,
    default: 'circle',
  },
  optionWidth: String,
  renderLabel: Function,
  renderLabelIcon: Function,
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

  selectOptions.value = list.map((item) => {
    if (isString(item)) {
      return { label: item, value: item };
    }
    return item;
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
  () => props.api,
  (newVal, oldVal) => {
    //bug：这里发生只api内存地址变化，实际api无变化也会触发监听。暂时使用深层对比解决
    if (!isEqual(newVal, oldVal)) {
      fetchData();
    }
  },
);

watch(selectOptions, (newVal) => {
  const { autoSelectedFirst, modelValue, valueKey } = props;

  // 自动选中第一项
  if (autoSelectedFirst && newVal.length && !modelValue) {
    emits('update:modelValue', newVal[0][valueKey]);
    selectChange(newVal[0][valueKey]);
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

watch(selectVal, (newVal, oldVal) => {
  selectChange(newVal);
});

const selectChange = (val) => {
  const { name, valueKey } = props;

  const item = selectOptions.value.find((item) => item[valueKey] === val);

  $selectData[name] = item;
};
</script>

<style lang="scss">
.radio-plus {
  .radio-button-item {
    &-icon {
      font-size: 25px;
      margin-bottom: 15px;
    }
    .el-radio-button__inner {
      width: v-bind(optionWidth);
    }
  }
}

.renderLabelIcon {
  .is-active {
    .el-radio-button__inner {
      background-color: transparent;
      color: var(--el-color-primary);
    }
  }
}
</style>
