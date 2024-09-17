<template>
  <div id="tableSelect">
    <h3>{{ title }}</h3>
    <Toolbar
      v-bind="props"
      :selected="state.selected"
      @refresh="fetchData"
      :mode="mode"
      :columns="currentColumns"
    />

    <el-table
      :data="state.list"
      v-loading="state.isLoading"
      height="100%"
      :default-sort="defaultSort"
      @sort-change="handleSortChange"
      @selection-change="handleSelectionChange"
      @current-change="handleCurrentChange"
      row-key="id"
      empty-text="暂无数据"
      ref="tableRef"
      :highlight-current-row="!multiple"
      show-overflow-tooltip
      :border="border"
      @filter-change="handleFilterChange"
    >
      <el-table-column
        type="selection"
        width="55"
        :reserve-selection="true"
        v-if="multiple && batchAction.length"
        :selectable="selectable"
      />

      <el-table-column width="40px" v-if="!multiple">
        <template #default="scope">
          <el-radio v-model="tableRadio" :label="scope.row">&nbsp;</el-radio>
        </template>
      </el-table-column>

      <el-table-column type="expand" v-if="expand">
        <template #default="props">
          <div class="expandContent">
            <component :is="expand(props.row)" />
          </div>
        </template>
      </el-table-column>

      <el-table-column
        v-for="{
          title,
          dataIndex,
          render,
          width,
          fixed,
          sortable,
          filters,
          filteredValue,
        } in currentColumns"
        :key="dataIndex"
        :prop="dataIndex"
        :label="title"
        :formatter="render"
        :width="width"
        :fixed="fixed"
        :sortable="sortable && 'custom'"
        :filters="filters"
        :column-key="dataIndex"
        :filter-multiple="false"
        :filtered-value="filteredValue"
      />

      <el-table-column
        fixed="right"
        :min-width="150"
        label="操作"
        v-if="rowAction.length"
      >
        <template #default="record">
          <el-space>
            <el-button
              v-for="{ title, disabled, onClick } in currentRowAction(
                record,
              ).slice(0, 2)"
              :key="title"
              :disabled="disabled"
              size="small"
              type="primary"
              link
              @click="onClick({ ...record.row })"
              >{{ title }}</el-button
            >

            <el-dropdown
              v-if="currentRowAction(record).length > 2"
              trigger="click"
            >
              <span>
                <el-button size="small" type="primary" link
                  >更多 <el-icon><CaretBottom /></el-icon
                ></el-button>
              </span>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item
                    :key="title"
                    v-for="{ title, onClick, disabled } in currentRowAction(
                      record,
                    ).slice(2)"
                  >
                    <el-button
                      size="small"
                      :key="title"
                      type="primary"
                      link
                      :disabled="disabled"
                      @click="onClick({ ...record.row })"
                      >{{ title }}</el-button
                    >
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </el-space>
        </template>
      </el-table-column>
    </el-table>

    <Pagination v-if="mode === 'remote'" :total="state.total" />
  </div>
</template>

<script setup lang="jsx">
import {
  defineProps,
  defineExpose,
  defineEmits,
  watch,
  reactive,
  onMounted,
  ref,
  computed,
  watchEffect,
  provide,
} from 'vue';
import request from '@/utils/request';
import { renderProps } from '@/utils';
import { CaretBottom } from '@element-plus/icons-vue';
import {
  merge,
  isEqual,
  pickBy,
  mapValues,
  isString,
  isFunction,
} from 'lodash';
import Toolbar from './Toolbar.vue';
import Pagination from './Pagination.vue';
import { useRoute, useRouter } from 'vue-router';

const props = defineProps({
  api: {
    default: () => ({
      url: '/api',
      method: 'get',
      data: {},
      params: {},
    }),
    type: Object,
  },
  columns: { default: () => [], type: Array },
  rowAction: {
    type: Array,
    default: () => [],
  },
  toolbarAction: {
    default: () => [],
    type: Array,
  },
  batchAction: {
    default: () => [],
    type: Array,
  },
  defaultSort: {
    default: () => ({ prop: 'createTime', order: 'descending' }),
    type: Object,
  },
  dataSource: {
    default: () => [],
    type: Array,
  },
  rowKey: {
    default: 'id',
    type: String,
  },
  //给外界回传已勾选数据
  modelValue: {
    default: () => [],
    type: Array,
  },
  //默认开启多选
  multiple: {
    default: true,
    type: Boolean,
  },
  //展开行（rowData)=>VNode
  expand: Function,
  //开启静态表格
  static: Boolean,
  //标题
  title: String,
  border: Boolean,
  filterKey: {
    default: 'filters',
    type: String,
  },
  selectFilterBar: {
    default: () => [],
    type: Array,
  },
  hasActionBar: {
    default: true,
    type: Boolean,
  },
  autoSelectedFirst: {
    type: Boolean,
    default: false,
  },
  name: String,
  selectable: {
    type: Function,
    default: () => true,
  },
});

const emit = defineEmits(['update:modelValue']);

const tableRef = ref(null);

const tableRadio = ref(null);

const state = reactive({
  isLoading: false,
  list: [],
  total: 0,
  selected: [],
  prevPageId: 0,
});

const route = useRoute();

const currentColumns = ref([]);

const currentParams = reactive(
  merge(props.api.data, {
    [props.filterKey]: {
      ...route.query,
    },
    pageRequest: {
      pageSize: 10,
      pageNo: 1,
      // lastID: 1,
    },
  }),
);

provide('currentParams', currentParams);

const asyncColumns = computed(async () => {
  const columns = props.columns.map(async (item) => {
    //获取列枚举值
    const filters = isFunction(item.filters)
      ? await item.filters()
      : item.filters;
    //列枚举回显
    const filteredValue = currentParams[props.filterKey][item.dataIndex]
      ? [currentParams[props.filterKey][item.dataIndex]]
      : [];

    return {
      ...item,
      filters,
      filteredValue,
    };
  });

  const newColumns = await Promise.all(columns);
  return newColumns;
});

const currentRowAction = computed(() => {
  const getRowAction = (record) => {
    let actions = props.rowAction.map((item) => ({
      ...item,
      hidden: renderProps(item.hidden, record.row),
      disabled: renderProps(item.disabled, record.row),
    }));

    return actions.filter((item) => !item.hidden);
  };

  return getRowAction;
});

const mode = computed(() => {
  if (props.static) {
    return 'static';
  }

  return 'remote';
});

const fetchData = async () => {
  const { url, method } = props.api;
  state.isLoading = true;

  // filters预处理，剔除假值，去除首位空格
  let currentFilters = pickBy(currentParams[props.filterKey], Boolean);
  currentFilters = mapValues(currentFilters, (value) =>
    isString(value) ? value.trim() : value,
  );

  const data = {
    ...currentParams,
    [props.filterKey]: currentFilters,
  };

  const { list } = await request({
    url,
    method,
    data,
  });

  const { total } = await request({
    url: url.replace('/search', '/total'),
    method,
    data,
  });

  if (Array.isArray(list)) {
    state.list = list;
  }

  if (total) {
    state.total = +total;
  }

  state.isLoading = false;
  state.selected = [];
};

watch(currentParams, () => {
  const { pageNum, pageSize } = currentParams;

  if (mode.value === 'static') {
    //使用静态分页
    state.list = props.dataSource.slice(
      (pageNum - 1) * pageSize,
      pageNum * pageSize,
    );
    state.total = props.dataSource.length;
  } else {
    fetchData();
  }
});

watch(
  () => props.api,
  (newVal, oldVal) => {
    //bug：这里发生只api内存地址变化，实际api无变化也会触发监听。暂时使用深层对比解决
    if (!isEqual(newVal, oldVal)) {
      merge(currentParams, newVal.data);
      fetchData();
    }
  },
);

const handleFilterChange = (filters) => {
  Object.keys(filters).forEach((key) => {
    Object.assign(currentParams[props.filterKey], { [key]: filters[key][0] });
  });
};

const handleSortChange = ({ prop, order }) => {
  if (prop && order) {
    currentParams.orderBys = { [prop]: order.replace('ending', '') };
  } else {
    currentParams.orderBys == {};
  }
};

const selectChange = (rows) => {
  state.selected = rows;
  emit('update:modelValue', rows);
};

//单选回调
const handleCurrentChange = (row) => {
  if (!props.multiple) {
    tableRadio.value = row;
    selectChange([row]);
  }
};
//多选回调
const handleSelectionChange = (rows) => {
  selectChange(rows);
};

watchEffect(() => {
  const { autoSelectedFirst, dataSource } = props;

  if (mode.value === 'static') {
    state.list = dataSource;
  }

  if (autoSelectedFirst && state.list.length && !state.selected.length) {
    const first = state.list[0];

    tableRef.value.setCurrentRow(first);
  }

  asyncColumns.value.then((data) => {
    currentColumns.value = data;
  });
});

onMounted(() => {
  if (mode.value === 'static') {
    state.list = props.dataSource;
    state.total = props.dataSource.length;
  } else {
    fetchData();
  }
});

//  抛出方法
defineExpose({ fetchData, currentParams });
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
