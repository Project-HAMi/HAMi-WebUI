<template>
  <el-tag closable v-bind="$attrs"><b>{{ label }}</b>：
    <component v-if="isVNode(parseValue)" :is="parseValue" />
    <span v-else>{{ parseValue }}</span>
  </el-tag>
</template>

<script setup lang="jsx">
import { computed, defineProps, isVNode } from 'vue';
import { isArray } from 'lodash';
import { ElPopover, ElTag } from 'element-plus';

const props = defineProps({
  label: String,
  value: null,
});

const parseValue = computed(() => {
  if (isArray(props.value)) {
    if (props.value.length <= 3) {
      return props.value.join('、');
    }

    return (
      <ElPopover trigger="hover">
        {{
          reference: () => <span>{props.value.length}个</span>,
          default: () => (
            <div
              style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}
            >
              {props.value.map((item) => (
                <ElTag type="info">{item}</ElTag>
              ))}
            </div>
          ),
        }}
      </ElPopover>
    );
  }
  const index = ['true', 'false'].indexOf(props.value);
  if (index == -1) return props.value;
  return index == 0 ? '是' : '否';
});
</script>
