<template>
  <div id="formGroup" v-if="!hide">
    <el-card
      v-if="component === 'card'"
      v-bind="componentProps"
      :header="label"
    >
      <slot />
    </el-card>

    <div v-if="component === 'divider'" style="margin-bottom: 60px">
      <el-divider style="margin-bottom: 40px">{{ label }}</el-divider>
      <slot />
    </div>

    <el-form-item
      id="form-item"
      :label-width="labelWidth"
      :label="label"
      v-if="component === 'inline'"
      class="inline"
    >
      <slot />
    </el-form-item>
    <div v-if="help && !hide" :style="{ marginLeft: labelWidth }" class="help-info">
      <el-icon color="#409EFF">
        <InfoFilled />
      </el-icon>
      {{ help }}
    </div>
    <div class="grid" v-if="component === 'grid'">
      <slot />
    </div>
  </div>
</template>

<script setup>
import { defineProps } from 'vue';
import { InfoFilled } from '@element-plus/icons-vue';
const props = defineProps({
  component: String,
  label: String,
  labelWidth: String,
  componentProps: Object,
  hide: Boolean,
  help: String,

});
</script>

<style lang="scss" scoped>
#formGroup {
  margin-bottom: 10px;
}
.inline {
  display: flex;
  width: 100%;
}

.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
}
</style>
