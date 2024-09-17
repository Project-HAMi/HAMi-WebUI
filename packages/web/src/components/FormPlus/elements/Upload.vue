<template>
  <div class="form-plus-upload" v-if="files.name">
    <el-input :model-value="files.name" readonly />
    <el-button @click="reset">重新选择</el-button>
  </div>
  <el-upload
    v-else
    v-model:file-list="value"
    :auto-upload="false"
    :show-file-list="false"
    drag
    v-bind="$attrs"
    class="form-plus-upload"
  >
    <el-icon class="el-icon--upload"><upload-filled /></el-icon>
    <div class="el-upload__text">将文件拖到此处 或 <em>点击上传</em></div>
    <!-- <template #tip>
      <div class="el-upload__tip">
        jpg/png files with a size less than 500kb
      </div>
    </template> -->
  </el-upload>
</template>

<script setup>
import { defineProps, defineEmits, computed, watchEffect, ref } from 'vue';
import { UploadFilled } from '@element-plus/icons-vue';

const props = defineProps({
  modelValue: {},
  multiple: Boolean,
});

const emit = defineEmits(['update:modelValue']);

const files = ref({});

const value = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emit('update:modelValue', val);

    if (!props.multiple) {
      files.value = val.at(-1);
    }
  },
});

const reset = () => {
  files.value = {};
};
</script>

<style lang="scss">
.form-plus-upload {
  display: flex;
  width: 400px;
  .el-upload {
    width: 100%;
  }
  .el-input__wrapper {
    height: 32px;
  }
}
</style>
