<template>
  <el-drawer
    v-model="visible"
    :title="title"
    :append-to-body="true"
    :size="width"
    direction="rtl"
    destroy-on-close
    id="FormPlusDrawer"
  >
    <form-plus
      v-model="form"
      :schema="schema"
      :schemaContext="schemaContext"
      ref="formPlusRef"
    />

    <slot />
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" @click="handleOk">确定</el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script setup>
import { ref, defineProps, defineEmits, computed } from 'vue';

const formPlusRef = ref(null);

const emit = defineEmits(['update:modelValue', 'update:form', 'ok']);

const props = defineProps({
  // 抽屉显隐，双向绑定
  modelValue: {
    type: Boolean,
    required: true,
  },
  // 表单数据源，双向绑定
  form: { type: Object, required: true },
  // 表单JSON配置
  schema: {
    type: Object,
    default: () => ({ items: [], initialValue: {} }),
  },
  // 抽屉宽度，以px为单位
  width: {
    type: Number,
    default: 550,
  },
  // 标题
  title: {
    type: String,
    default: '',
  },
  schemaContext: Object,
});

const visible = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emit('update:modelValue', val);
  },
});

const form = computed({
  get() {
    return props.form;
  },
  set(val) {
    emit('update:form', val);
  },
});

const handleOk = async () => {
  const values = await formPlusRef.value.submit();
  emit('ok', values, formPlusRef.value.selectData);
};
</script>

<style lang="scss">
#FormPlusDrawer {
  .el-drawer__header {
    border-bottom: 1px solid #eee;
    margin-bottom: 0;
    padding-bottom: 32px;
  }
  .el-drawer__title {
    overflow: hidden;
    color: #1d2b3a;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: 'PingFang SC';
    font-size: 16px;
    font-style: normal;
    font-weight: 500;
    line-height: 24px; /* 150% */
  }
  .el-drawer__footer {
    // background-color: #eee;
    padding: 16px 20px;
    .dialog-footer {
      text-align: right;
    }
  }
}
</style>
