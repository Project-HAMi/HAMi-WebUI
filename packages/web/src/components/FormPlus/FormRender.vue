<template>
  <template v-for="item in formItems">
    <form-group
      v-if="item.children"
      v-bind="item"
      :label="item.label"
      :component="item.component"
      :label-width="labelWidth"
      :componentProps="item.props"
      :key="item.name"
    >
      <template v-for="c in item.children">
        <form-group
          v-if="c.children"
          :label="c.label"
          :component="c.component"
          :label-width="labelWidth"
          :key="c.name"
          :componentProps="c.props"
          :help="c.help"
        >
          <FormRender
            :labelWidth="labelWidth"
            v-model="form"
            :formItems="c.children"
            :key="c.label"
          />
        </form-group>

        <form-item
          v-else
          v-model="formValues[c.name]"
          v-bind="c"
          :label-width="labelWidth"
          :componentProps="c.props"
          :key="c.label"
        />
      </template>
    </form-group>

    <form-item
      v-else
      v-model="formValues[item.name]"
      v-bind="item"
      :label-width="labelWidth"
      :componentProps="item.props"
      :key="item.label"
    />
  </template>
</template>

<script setup>
import { defineProps, defineEmits, computed } from 'vue';
import FormItem from './FormItem.vue';
import FormGroup from './FormGroup.vue';

const props = defineProps({
  labelWidth: String,
  modelValue: Object,
  formItems: Array,
});

const emit = defineEmits(['update:modelValue']);

//原form总数据源
const form = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emit('update:modelValue', val);
  },
});
// 通过Proxy接管的数据源，某项属性被修改会立刻通知父组件，遵守单项数据流原则
const formValues = computed(() => {
  return new Proxy(props.modelValue, {
    set(target, key, value) {
      emit('update:modelValue', { ...target, [key]: value });
      return true;
    },
  });
});
</script>
