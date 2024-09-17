<template>
  <div id="filterInput">
    <div class="ico">
      <svg-icon icon="search" />
    </div>
    <div class="filterTags" v-if="filterTags.length">
      <el-space>
        <el-tag
          v-for="{ text, value, q } in filterTags"
          :key="value"
          closable
          @close="closeFilter(value)"
        >
          {{ text }}:{{ q }}
        </el-tag>
      </el-space>
    </div>
    <div class="input" @click="openSearchDropdown">
      <el-dropdown
        ref="searchDropdownRef"
        trigger="contextmenu"
        placement="bottom-start"
        style="width: 100%"
      >
        <el-input
          v-model="searchVal"
          placeholder="请输入搜索关键词"
          @keyup.enter="handleSearch"
          clearable
          @focus="openSearchDropdown"
          ref="searchInputRef"
        />
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item
              :key="item.value"
              @click="handleSelectSearchType(item)"
              v-for="item in filterOptions"
              >{{ item.text }}</el-dropdown-item
            >
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>
</template>

<script setup>
import { defineProps, ref, computed, defineEmits } from 'vue';
import { ElMessage } from 'element-plus';
import { omit } from 'lodash';

const props = defineProps({
  modelValue: Object, //数据源，双向绑定
  options: { default: () => [], type: Array }, //下拉提示字段数据
});

const searchInputRef = ref(null);

const searchDropdownRef = ref(null);

const searchVal = ref('');

const twoOptions = ref([]);

const emit = defineEmits(['update:modelValue']);

const filterOptions = computed(() => {
  if (twoOptions.value.length) return twoOptions.value;

  const filterTagsKeys = filterTags.value.map((item) => item.value);

  return props.options.filter((item) => !filterTagsKeys.includes(item.value));
});

const filterTags = computed(() => {
  const { modelValue, options } = props;

  return options
    .map((item) => {
      let q = modelValue[item.value];

      //如果配了枚举值，就查找转换
      if (item.filters) {
        q =
          item.filters.find((item) => {
            return item.value === q;
          })?.text || q;
      }

      return {
        ...item,
        q,
      };
    })
    .filter((item) => !!item.q);
});

const handleSearch = () => {
  twoOptions.value = [];

  const [text, textQ] = searchVal.value.split(':');

  if (textQ) {
    // 找到当前输入 对应得数据源
    const source = filterOptions.value.find((item) => item.text === text);
    // 转化key
    const type = source.value;
    // 转化value
    const value =
      source.filters?.find((item) => item.text === textQ)?.value || textQ;
    emit('update:modelValue', { ...props.modelValue, [type]: value });
  } else {
    ElMessage.error('输入不合法');
  }

  searchVal.value = '';
};

const handleSelectSearchType = (item) => {
  if (twoOptions.value.length) {
    searchVal.value = searchVal.value + item.text;
    handleSearch();
  } else {
    searchVal.value = item.text + ':';
    searchInputRef.value.focus();
  }

  if (item.filters) {
    //如果有filters,展开二级筛选菜单
    twoOptions.value = item.filters;
    searchDropdownRef.value.handleOpen();
  }
};

const closeFilter = (key) => {
  emit('update:modelValue', omit(props.modelValue, [key]));
};

const openSearchDropdown = () => {
  if (!searchVal.value || twoOptions.value.length) {
    searchDropdownRef.value.handleOpen();
  }
  if (!searchVal.value) {
    twoOptions.value = [];
  }
};

const onClearSearch = () => {
  searchVal.value = '';
};
</script>

<style lang="scss">
#filterInput {
  margin-bottom: 10px;
  display: flex;
  flex: 1;
  padding: 2px 0;
  border-radius: 5px;
  color: #606266;

  box-sizing: border-box;
  background-color: #f5f7fa;
  box-shadow: 0 0 0 1px #e4e7ed inset;
  .ico {
    padding: 0 10px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #606266;
  }
  .filterTags {
    display: flex;
  }

  .searchtext {
    padding: 0 10px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .input {
    position: relative;
    flex: 1;
  }
  .el-input__wrapper {
    box-shadow: none;
    background-color: transparent;
    padding-left: 0;
  }
}
</style>
