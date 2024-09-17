<template>
  <el-form-item
    v-if="!hide"
    id="form-item"
    :key="name"
    :prop="name"
    :label="!hideLabel && !schema?.hideLabel && label"
    :label-width="
      !hideLabel && !schema?.hideLabel && label ? schema.labelWidth : '0'
    "
    :rules="computeRules"
    v-bind="$attrs"
  >
    <template #label v-if="!hideLabel">
      <div
        class="form-item-label"
        style="display: flex; gap: 5px; align-items: center"
      >
        <div>{{ label }}</div>
        <div class="ico" v-if="tip">
          <el-tooltip class="box-item" effect="dark" :content="tip">
            <div><svg-icon icon="help" /></div>
          </el-tooltip>
        </div>
      </div>
    </template>

    <el-input
      v-model="value"
      autocomplete="off"
      placeholder="请输入"
      v-if="component === 'input'"
      v-bind="componentProps"
      class="form-item-input"
    />

    <el-input
      v-model="value"
      autocomplete="off"
      v-if="component === 'password'"
      show-password
      v-bind="componentProps"
      type="password"
      class="form-item-input"
    />

    <el-input
      v-model="value"
      autocomplete="off"
      v-bind="componentProps"
      :autosize="{ minRows: 4, maxRows: 999 }"
      type="textarea"
      v-if="component === 'textarea'"
      class="form-item-input"
    />

    <number-input
      v-model="value"
      v-bind="componentProps"
      v-if="component === 'inputNumber'"
    />

    <select-plus
      v-model="value"
      v-bind="componentProps"
      v-if="component === 'select'"
      :name="name"
    />

    <radio-plus
      v-model="value"
      v-bind="componentProps"
      v-if="component === 'radio'"
      :name="name"
    />

    <el-color-picker
      v-model="value"
      v-if="component === 'colorPicker'"
      v-bind="componentProps"
    />

    <form-list
      v-model="value"
      v-if="component === 'formList'"
      v-bind="componentProps"
    />

    <table-select
      v-model="value"
      v-if="component === 'selectTable'"
      v-bind="componentProps"
      :name="name"
    />

    <el-switch
      v-model="value"
      v-if="component === 'switch'"
      v-bind="componentProps"
    />

    <item-group
      v-model="value"
      v-if="component === 'itemGroup'"
      :labelWidth="labelWidth"
      v-bind="componentProps"
    />

    <time-picker
      v-model="value"
      v-if="component === 'timePicker'"
      v-bind="componentProps"
    />

    <Cascader
      v-model="value"
      v-if="component === 'cascader'"
      v-bind="componentProps"
    />

    <Upload
      v-model:file-list="value"
      v-if="component === 'upload'"
      v-bind="componentProps"
    ></Upload>

    <div v-if="component === 'text'">
      {{ componentProps.formatter || value }}
    </div>

    <slider-plus
      v-model="value"
      v-if="component === 'sliderPlus'"
      v-bind="componentProps"
      :name="name"
    />

    <Transfer
      v-model="value"
      v-if="component === 'transfer'"
      v-bind="componentProps"
      :name="name"
    />

    <el-tag v-if="component === 'tag'" v-bind="componentProps">{{
      componentProps.text
    }}</el-tag>

    <el-alert v-if="component === 'alert'" v-bind="componentProps"></el-alert>

    <el-tree-select
      v-model="value"
      check-strictly
      :render-after-expand="false"
      style="width: 240px"
      v-if="component === 'treeSelect'"
      v-bind="componentProps"
    />
  </el-form-item>
  <div v-if="help && !hide" :style="{ marginLeft: labelWidth }" class="help-info">
    <el-icon color="#409EFF">
      <InfoFilled />
    </el-icon>
    {{ help }}
  </div>
</template>

<script setup>
import { InfoFilled } from '@element-plus/icons-vue';
import {
  computed,
  defineProps,
  defineEmits,
  onMounted,
  inject,
  watchEffect,
} from 'vue';
import FormList from './FormList.vue';
import TableSelect from './TableSelect';
import Cascader from './elements/Cascader.vue';
import Upload from './elements/Upload.vue';
import Transfer from './elements/Transfer.vue';

const data = [
  {
    value: '1',
    label: 'Level one 1',
    children: [
      {
        value: '1-1',
        label: 'Level two 1-1',
        children: [
          {
            value: '1-1-1',
            label: 'Level three 1-1-1',
          },
        ],
      },
    ],
  },
  {
    value: '2',
    label: 'Level one 2',
    children: [
      {
        value: '2-1',
        label: 'Level two 2-1',
        children: [
          {
            value: '2-1-1',
            label: 'Level three 2-1-1',
          },
        ],
      },
      {
        value: '2-2',
        label: 'Level two 2-2',
        children: [
          {
            value: '2-2-1',
            label: 'Level three 2-2-1',
          },
        ],
      },
    ],
  },
  {
    value: '3',
    label: 'Level one 3',
    children: [
      {
        value: '3-1',
        label: 'Level two 3-1',
        children: [
          {
            value: '3-1-1',
            label: 'Level three 3-1-1',
          },
        ],
      },
      {
        value: '3-2',
        label: 'Level two 3-2',
        children: [
          {
            value: '3-2-1',
            label: 'Level three 3-2-1',
          },
        ],
      },
    ],
  },
];

const props = defineProps({
  label: String,
  name: String,
  component: String,
  required: Boolean,
  componentProps: Object,
  labelWidth: String,
  help: String,
  modelValue: null,
  initialValue: null,
  rules: Array,
  hideLabel: Boolean,
  hide: Boolean,
  tip: String,
});

const emit = defineEmits(['update:modelValue']);

const value = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emit('update:modelValue', val);
  },
});

const schema = inject('$schema', {});

const computeRules = computed(() => {
  const { rules, required } = props;

  const ruleData = [];

  if (required) {
    ruleData.push({
      required: true,
      message: '该字段是必填字段',
      trigger: 'blur',
    });
  }

  if (rules) {
    const ruleParse = rules.map(
      ({ type, message, trigger, reg, validator }) => {
        const ruleDef = {
          message,
          trigger,
        };
        if (type) {
          return { ...ruleDef, type };
        }
        if (reg) {
          return {
            ...ruleDef,
            pattern: reg,
          };
        }
        if (validator !== undefined) {
          return {
            ...ruleDef,
            validator: () => validator,
          };
        }

        return {};
      },
    );
    return [...ruleData, ...ruleParse];
  }

  return ruleData;
});

onMounted(() => {
  setTimeout(() => {
    if (props.initialValue) {
      emit('update:modelValue', props.initialValue);
    }
  });
});
</script>

<style lang="scss">
#form-item {
  .el-form-item__label {
    font-weight: bold;

    &::before {
      display: none;
    }
  }

  .form-item-input,
  .el-select {
    max-width: 400px;
    min-width: 195px;
  }
}

.help-info {
  background-color: #f7f7f7;
  padding: 2px 10px;
  line-height: 25px;
  position: relative;
  top: -8px;
}
</style>
