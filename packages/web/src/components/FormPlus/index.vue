<template>
  <el-form
    :model="form"
    :labelPosition="schema.labelAlign || 'left'"
    :style="`max-width: ${schema.formWidth}`"
    :class="schema.class"
    ref="formRef"
  >
    <FormRender
      :labelWidth="schema.labelWidth"
      v-model="form"
      :formItems="formItems"
    />
  </el-form>
</template>

<script setup>
import {
  ref,
  defineProps,
  defineExpose,
  defineEmits,
  computed,
  reactive,
  provide,
  watch,
  watchEffect,
  onMounted,
} from 'vue';
import { ElMessage } from 'element-plus';
import { timeParse, parseMemory } from '@/utils';
import { useStore } from 'vuex';
import { parseFormItems, handleLinkages } from './utils';
import FormRender from './FormRender.vue';

const formRef = ref(null);

const store = useStore();

const props = defineProps({
  // 表单数据源，双向绑定
  modelValue: {
    type: Object,
  },
  // 表单JSON配置
  schema: {
    type: Object,
    default: () => ({ items: [] }),
  },
  schemaContext: {
    type: Object,
    default: () => ({ items: [] }),
  },
});

const emit = defineEmits(['update:modelValue']);

const form = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emit('update:modelValue', val);
  },
});

const selectData = reactive({});

provide('$selectData', selectData);

// 转换为动态配置
const formItems = computed(() => {
  const context = {
    $values: form.value,
    $form: form.value,
    $selectData: selectData,
    $utils: {
      timeParse,
      parseMemory,
    },
    $global: store.state.global,
    $userInfo: store.state.user,
    ...props.schemaContext,
  };
  return parseFormItems(props.schema.items, context);
});

const submit = async () => {
  try {
    await formRef.value.validate();
    //转化成普通对象，便于阅读
    return { ...form.value };
  } catch (e) {
    ElMessage.error('表单填写校验不通过！');
    return Promise.reject(e);
  }
};

const $schema = computed(() => props.schema);

provide('$schema', $schema);

watch(
  form,
  (newVal, oldVal) => {
    handleLinkages({ newVal, oldVal, form, formItems: formItems.value });
  },
  { deep: true },
);
onMounted(() => {
});
defineExpose({ submit, selectData, formItems });
</script>
