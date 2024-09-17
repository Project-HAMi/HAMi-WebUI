<template>
  <div class="table-toolbar"  @load="emitLoadEvent">
    <!-- 高级搜索 -->
    <Transition
      enter-active-class="animate__animated animate__fadeIn"
      leave-active-class="animate__animated animate__fadeOut"
    >
      <div class="searchForm" v-show="searchFormVisible">
        <form-plus
          v-model="searchFormValues"
          :schema="searchSchema"
          inline
          ref="searchFormRef"
        />
      </div>
    </Transition>

    <!-- 钉 -->
    <div class="search-tags" v-if="currentTags.length && !hideTag">
      <SearchTag
        v-for="[key, value] in currentTags"
        :key="key"
        :label="formItemConfigs[key]?.label || key"
        :value="formItemConfigs[key]?.value || value"
        @close="
          formItemConfigs[key]?.label === '组织' ? null : handleClearTag(key)
        "
      />
    </div>

    <!-- 操作按钮 -->
    <div class="actionBar" v-if="hasActionBar">
      <div class="left">
        <div class="batchButton" v-if="batchAction.length">
          <el-space>
            <el-button
              v-for="{
                title,
                disabled,
                type,
                icon,
                onClick,
                hidden,
              } in batchAction"
              v-show="!hidden"
              :key="title"
              :type="type || 'primary'"
              :disabled="!selected.length || renderProps(disabled, selected)"
              :icon="icon"
              @click="onClick(selectedData)"
              >{{ title }}</el-button
            >
          </el-space>
        </div>
      </div>

      <div :class="['right', { hasBatchButton: batchAction.length }]">
        <div class="toolbarAction">
          <el-button
            :type="type"
            :icon="icon"
            @click="onClick"
            :key="name"
            v-for="{ name, type, icon, disabled, onClick } in toolbarAction"
            :disabled="renderProps(disabled)"
            >{{ name }}</el-button
          >
        </div>

        <div class="toolButton">
          <!-- 搜索框 -->
          <div class="searchInput">
            <!--          <el-input-->
            <!--            v-model="searchNameVal"-->
            <!--            placeholder="按名称模糊搜索"-->
            <!--            @keyup.enter="handleSearchName"-->
            <!--          >-->
            <!--            <template #suffix>-->
            <!--              <svg-icon icon="search" />-->
            <!--            </template>-->
            <!--          </el-input>-->

            <el-button
              v-if="searchSchema.items.length"
              @click="searchFormVisible = !searchFormVisible"
            >
              <template #icon>
                <svg-icon icon="highSearch" />
              </template>
              高级
            </el-button>
          </div>

          <el-dropdown trigger="click">
            <el-button>
              <template #icon>
                <svg-icon icon="eye" />
              </template>
            </el-button>
            <template #dropdown>
              <el-checkbox-group style="padding: 10px" v-model="eyeKeys">
                <el-space direction="vertical" alignment="flex-start">
                  <el-checkbox
                    :key="title"
                    :label="title"
                    :value="title"
                    v-for="{ title } in columns"
                  />
                </el-space>
              </el-checkbox-group>
            </template>
          </el-dropdown>

          <el-button @click="handleRefresh" v-if="mode === 'remote'">
            <template #icon>
              <svg-icon icon="refresh" />
            </template>
          </el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="jsx">
import {
  defineProps,
  inject,
  computed,
  defineEmits,
  ref,
  watchEffect,
  onMounted,
  defineExpose,
  watch
} from 'vue';
import { Refresh } from '@element-plus/icons-vue';
import { renderProps } from '@/utils';
import { pick, pickBy, isArray } from 'lodash';
import getFields from '@/utils/getFields';
import { deepParse } from '@/utils/form';
import SearchTag from './SearchTag.vue';

const props = defineProps({
  mode: String,
  columns: { default: () => [], type: Array },
  toolbarAction: {
    default: () => [],
    type: Array,
  },
  batchAction: {
    default: () => [],
    type: Array,
  },
  filterKey: { default: 'filters', type: String },
  selectFilterBar: {
    default: () => [],
    type: Array,
  },
  hasActionBar: {
    default: true,
    type: Boolean,
  },
  selected: {
    default: () => [],
    type: Array,
  },
  rowKey: {
    default: 'id',
    type: String,
  },
  searchSchema: {
    type: Object,
    default: () => ({ items: [] }),
  },
  eyeColumnKeys: {
    default: () => [],
    type: Array,
  },
  defaultVisibleSearchForm: Boolean,
  hideTag: Boolean,
});

const emit = defineEmits(['refresh', 'update:eyeColumnKeys','load']);

const currentParams = inject('currentParams');

const searchFormRef = ref(null);

const searchFormVisible = ref(props.defaultVisibleSearchForm);

const searchNameVal = ref('');
const eyeKeys = computed({
  get() {
    return props.eyeColumnKeys;
  },
  set(keys) {
    emit('update:eyeColumnKeys', keys);
  },
});

// const parseSearchSchema=computed(()=>deepParse(props.searchSchema,{}))

const searchFormValues = computed({
  get() {
    try {
      const searchCache = JSON.parse(localStorage.getItem('searchCache'));
      const searchFields = props.searchSchema?.items || [];
      const searchFieldkeys = searchFields.map((item) => item.name);
      return {
        ...currentParams.filters,
        ...pick(searchCache, searchFieldkeys),
      };
    } catch {
      return currentParams.filters;
    }
    
  },
  set(values) {
    const newCache = pick(values, [
      'providerID',
      'regionID',
      'zoneIDs',
      'businessIDs',
      'appIDs',
    ]);

    try {
      const oldCache = localStorage.getItem('searchCache');

      localStorage.setItem(
        'searchCache',
        JSON.stringify({
          ...JSON.parse(oldCache),
          ...newCache,
        }),
      );
    } catch {
      localStorage.setItem('searchCache', JSON.stringify(newCache));
    }
    currentParams.filters = values;
  },
});

const formItemConfigs = computed(() => {
  const items = searchFormRef.value?.formItems;
  return getFields(items, {
    $form: searchFormValues.value,
    $selectData: searchFormRef.value?.selectData || {},
  });
});

const currentTags = computed(() => {
  let tagList = Object.entries(searchFormValues.value).filter(
    ([key, value]) => {
      if (isArray(value) && !value.length) {
        return false;
      }
      if (!formItemConfigs.value[key]) {
        return false;
      }

      if (!value) {
        return false;
      }
      return true;
    },
  );

  return tagList.sort((a, b) => {
    const itemAOrder = formItemConfigs.value[a[0]]?.order;
    const itemBOrder = formItemConfigs.value[b[0]]?.order;

    return itemAOrder - itemBOrder;
  });
});

const handleClearTag = (key) => {
  delete currentParams.filters[key];
  try {
    const searchCache = JSON.parse(localStorage.getItem('searchCache'));
    delete searchCache[key];

    localStorage.setItem('searchCache', JSON.stringify(searchCache));
  } catch {}
};

const selectedData = computed(() => {
  return {
    rows: props.selected,
    keys: props.selected.map((item) => item[props.rowKey]),
  };
});

const handleSearchName = () => {
  searchFormValues.value = {
    ...searchFormValues.value,
    name: searchNameVal.value,
  };
};

const handleRefresh = () => {
  emit('refresh');
};

onMounted(() => {
  // if (Object.keys(searchFormValues.value).length) {
  //   currentParams.filters = searchFormValues.value;
  // }
  emit('load')
});

defineExpose({ searchFormValues });
</script>

<style lang="scss">
.table-toolbar {
  /* display: flex; */
  margin-bottom: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;

  .searchForm {
    animation-duration: 0.3s;
    // border-bottom: 1px solid #eee;
  }

  .selected {
    margin-right: 15px;
  }

  .searchInput {
    display: flex;
    gap: 5px;
  }

  .search-tags {
    // border-bottom: 1px solid #eee;
    display: flex;
    gap: 3px;
    align-items: center;
    flex-wrap: wrap;
  }

  .toolButton {
    display: flex;
    gap: 5px;
  }

  button {
    height: 100%;
  }
}

.el-form--inline .el-form-item {
  margin-right: 10px;
  margin-bottom: 5px;
}
</style>
