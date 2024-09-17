<template>
  <div id="formList">
    <template v-if="mode === 'inline'">
      <el-form-item v-for="(item, index) in list" :key="index" class="list-item">
        <el-space>
          <form-item
            v-for="field in fields"
            v-model="item[field.name]"
            v-bind="field"
            :componentProps="field.props"
            :key="field.label"
          />
        </el-space>
        <el-button
          v-if="allowReduce"
          @click="reduce({ row: item, $index: index })"
          :icon="Minus"
          circle
          type="primary"
          class="list-btn"
        ></el-button>
      </el-form-item>
    </template>

    <template v-if="mode === 'card'">
      <template v-for="(item, index) in list" :key="index">
        <el-tag v-if="or && index > 0">或</el-tag>

        <el-card class="list-card">
          <template #header>
            <div class="card-header">
              <span v-if="showIndex"> {{ title + (index + 1) }}</span>
              <span v-else>{{ title }}</span>
              <el-button
                v-if="allowReduce"
                @click="reduce({ row: item, $index: index })"
                circle
                type="primary"
                class="list-btn"
                :icon="Minus"
              >
                <template #icon>
                  <icon-render name="reduce" color="#fff" />
                </template>
              </el-button>
            </div>
          </template>
          <form-item
            v-for="field in formatterFields(index)"
            v-bind="field"
            :key="field.label"
            class="list-card-item"
            :componentProps="field.props"
            v-model="item[field.name]"
            :style="{ marginBottom: '15px' }"
          />
        </el-card>
      </template>
    </template>

    <el-table v-if="mode === 'table'" :data="list" style="width: 100%">
      <el-table-column
        v-for="(item,index) in fields"
        :prop="item.name"
        :label="item.label"
        :key="index"
        :width="item.width"
        :formatter="(row, _, __, index) => formatter(item, row, index)"
      >
        <template #header>
          {{ item.label }}
          <el-tooltip
            effect="dark"
            content="以 art. 为前缀的属性不支持修改和删除"
            placement="top"
            v-if="item.tip"
          >
            <svg-icon icon="tip" />
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column fixed="right" min-width="60">
        <template #default="record">
          <el-button
            v-if="allowReduce"
            @click="reduce(record)"
            circle
            type="primary"
            class="list-btn"
            :icon="Minus"
            :disabled="record.row.immutable"
          >
          </el-button>
        </template>
      </el-table-column>

      <template #append>
        <div class="table-append">
          <el-button v-if="allowAdd && !isMax" @click="add" :icon="Plus"
            >增加</el-button
          >
        </div>
      </template>
    </el-table>

    <div v-if="mode !== 'table'">
      <el-button
        v-if="allowAdd && !isMax"
        @click="add"
        :icon="Plus"
        type="primary"
        class="list-btn addBtn"
        :circle="!addTitle"
        >{{ addTitle }}</el-button
      >
    </div>
  </div>
</template>

<script setup>
import {
  computed,
  defineProps,
  defineEmits,
  onMounted,
  h,
  ref,
  defineExpose,
} from 'vue';
import { Plus, Minus } from '@element-plus/icons-vue';
import { deepParse } from '@/utils/form';
import FormItem from './FormItem.vue';

const props = defineProps({
  modelValue: Array,
  fields: Array,
  allowAdd: {
    default: true,
    type: Boolean,
  },
  allowReduce: {
    default: true,
    type: Boolean,
  },
  defaultLineCount: {
    default: 0,
    type: Number,
  },
  minLines: {
    default: 0,
    type: Number,
  },
  maxLines: {
    default: 999,
    type: Number,
  },
  mode: {
    default: 'inline',
    type: String,
  },
  title: {
    type: String,
    default: '',
  },
  showIndex: {
    type: Boolean,
    default: true,
  },
  rowKey: {
    type: String,
    default: 'key',
  },
  addTitle: {
    type: String,
    default: '增加',
  },
  or: Boolean,
});

const emit = defineEmits(['update:modelValue']);

const removeData = ref([]);

const list = computed({
  get() {
    return props.modelValue || [];
  },
  set(val) {
    emit('update:modelValue', val);
  },
});

const isMax = computed(() => {
  return list.value.length >= props.maxLines;
});

const add = () => {
  if (isMax.value) {
    return;
  }
  emit('update:modelValue', [...list.value, {}]);
};

const reduce = ({ $index, row }) => {
  removeData.value.push(row);
  if (props.minLines === list.value.length) return;
  const newData = list.value.filter((v, index) => index !== $index);
  emit('update:modelValue', newData);
};

const formatter = (oldItem, data, index) => {
  const item = deepParse(oldItem, { $item: data, $currentIndex: index, $currentItem: data });
  return h(FormItem, {
    ...item,
    hideLabel: true,
    style: { marginBottom: 0 },
    modelValue: data[item.name],
    'onUpdate:modelValue': (newValue) => (data[item.name] = newValue),
    componentProps: {
      ...(item.props || {}),
      // disabled: data.immutable,
    },
  });
};

const formatterFields = computed(
  () => (index) =>
    deepParse(props.fields, { $item: list.value[index], $index: index })
);

onMounted(() => {
  if (props.minLines && !list.value.length) {
    list.value = Array.from({ length: props.minLines }, () => {
      return {};
    });
  }
});

defineExpose({ removeData });
</script>

<style lang="scss" scoped>
#formList {
  position: relative;
  width: 100%;
  .list-item {
    margin-bottom: 10px;
  }
  .list-btn {
    margin-left: 10px;
  }

  .addBtn {
  }

  .list-card {
    margin-bottom: 10px;
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .list-card-item {
      margin-bottom: 15px;
    }
  }

  .table-append {
    border-top: 1px solid #eee;
    padding: 10px;
  }
}
</style>
