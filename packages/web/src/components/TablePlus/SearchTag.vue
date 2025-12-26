<template>
  <el-tag closable v-bind="$attrs"><b>{{ label }}</b>：
    <component v-if="isVNode(parseValue)" :is="parseValue" />
    <span v-else>{{ parseValue }}</span>
  </el-tag>
</template>

<script setup lang="jsx">
import { computed, isVNode } from 'vue';
import { isArray } from 'lodash';
import { ElPopover, ElTag } from 'element-plus';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

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
          reference: () => <span>{props.value.length}{t('common.unitCount')}</span>,
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
  return index == 0 ? t('common.yes') : t('common.no');
});
</script>
