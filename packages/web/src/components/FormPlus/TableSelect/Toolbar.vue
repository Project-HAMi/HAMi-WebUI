<template>
  <div class="grid-table-toolbar">
    <div class="searchBar" v-if="mode === 'remote'">
      <div class="selectSearchBar">
        <select-plus
          style="max-width: 120px"
          v-bind="item"
          v-for="item in selectFilterBar"
          :key="item.filterKey"
          v-model="currentParams[filterKey][item.filterKey]"
        />
      </div>
      <filter-input
        :options="searchOptions"
        v-model="currentParams[filterKey]"
      />
    </div>

    <div class="actionBar" v-if="hasActionBar">
      <div class="batchButton">
        <el-space>
          <el-button
            v-for="{ title, disabled, type, icon, onClick } in batchAction"
            :key="title"
            :type="type || 'primary'"
            plain
            :disabled="!selected.length || renderProps(disabled, selected)"
            :icon="icon"
            @click="onClick(selectedData)"
            >{{ title }}</el-button
          >
        </el-space>
      </div>

      <div class="toolButton">
        <el-button
          :type="type"
          size="small"
          :icon="icon"
          @click="onClick"
          :key="name"
          v-for="{ name, type, icon, disabled, onClick } in toolbarAction"
          :disabled="renderProps(disabled)"
          >{{ name }}</el-button
        >
        <el-button
          type="primary"
          :icon="Refresh"
          @click="handleRefresh"
          v-if="mode === 'remote'"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="jsx">
import { defineProps, inject, computed, defineEmits } from 'vue';
import { Refresh, CaretBottom } from '@element-plus/icons-vue';
import { renderProps } from '@/utils';

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
});

const emit = defineEmits(['refresh']);

const currentParams = inject('currentParams');

const searchOptions = computed(() => {
  return props.columns
    .filter((item) => item.search)
    .map((item) => ({
      text: item.title,
      value: item.dataIndex,
      filters: item.filters,
    }));
});

const selectedData = computed(() => {
  return {
    rows: props.selected,
    keys: props.selected.map((item) => item[props.rowKey]),
  };
});

const handleRefresh = () => {
  emit('refresh');
};
</script>

<style lang="scss">
#tableSelect {
  display: flex;
  height: 100%;
  width: 100%;
  flex-direction: column;
  position: relative;
  background-color: #fff;
  padding: 10px;
  box-sizing: border-box;
  border-radius: 5px;

  h3 {
    margin-bottom: 0;
  }

  .pagination {
    margin-top: 15px;
    justify-content: right;
  }

  .expandContent {
    padding: 0 20px;
    // padding-left: 100px;
    // padding-bottom: 20px;
  }

  h2 {
    margin: 0 15px 15px;
  }

  .grid-table-toolbar {
    /* display: flex; */
    margin-bottom: 5px;
    .searchBar {
      display: flex;
    }
    .selected {
      margin-right: 15px;
    }

    .actionBar {
      display: flex;
      justify-content: space-between;
    }

    .toolButton {
      display: flex;
    }

    button {
      height: 100%;
    }
  }

  .cell {
    text-align: center;
  }
}
</style>
