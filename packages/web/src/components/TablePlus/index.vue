<template>
  <div class="table-plus">
    <slot name="title" />

    <div class="grid-table">
      <div class="table-content">
        <Toolbar
          v-bind="props"
          :selected="state.selected"
          @refresh="fetchData"
          :mode="mode"
          :columns="columns"
          :searchSchema="searchSchema"
          v-model:eyeColumnKeys="eyeColumnKeys"
          v-if="!static"
          ref="toolbarRef"
          :hideTag="hideTag"
        />

        <el-table
          class="table-main"
          :data="state.list"
          v-loading="state.isLoading"
          @sort-change="handleSortChange"
          @selection-change="handleSelectionChange"
          @current-change="handleCurrentChange"
          row-key="id"
          empty-text="暂无数据"
          ref="tableRef"
          :highlight-current-row="!multiple"
          :border="border"
          @filter-change="handleFilterChange"
          v-bind="$attrs"
          @row-click="rowClick"
          :row-class-name="rowCalssName"
          :header-cell-style="{ background: '#999999' }"
          :cell-class-name="cellClassName"
          :cell-style="cellStyle"
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
              <el-radio v-model="tableRadio" :label="scope.row"
                >&nbsp;</el-radio
              >
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
              sort,
              filters,
              hideTooltip,
              filteredValue,
              renderHeader,
            } in columns.filter((item) => eyeColumnKeys.includes(item.title))"
            :key="dataIndex"
            :prop="dataIndex"
            :label="title"
            :formatter="render"
            :width="width"
            :fixed="fixed"
            :sortable="sort && 'custom'"
            :filters="isFunction(filters) ? filters(state.list) : filters"
            :column-key="dataIndex"
            :filter-multiple="false"
            :filtered-value="filteredValue"
            :show-overflow-tooltip="!hideTooltip"
          >
            <template #header v-if="renderHeader">
              <component :is="renderHeader()" />
            </template>
          </el-table-column>

          <el-table-column
            fixed="right"
            :min-width="180"
            label="操作"
            v-if="rowAction.length"
          >
            <template #default="record">
              <div class="rowActions">
                <el-button
                  v-for="{ title, disabled, onClick } in currentRowAction(
                    record,
                  ).slice(0, 2)"
                  :key="title"
                  :disabled="disabled"
                  size="small"
                  type="primary"
                  text
                  @click.stop="onClick({ ...record.row }, record.$index)"
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
                          @click="onClick({ ...record.row }, record.$index)"
                          >{{ title }}</el-button
                        >
                      </el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
      <Pagination
        v-if="hasPagination && mode === 'remote'"
        :total="state.total"
      />
    </div>
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
  debounce,
  isFunction,
} from 'lodash';
import Toolbar from './Toolbar.vue';
import Pagination from './Pagination.vue';
import { useRoute } from 'vue-router';
import Sortable from 'sortablejs';
import './style.scss';

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
    default: () => ({ sort: 'DESC', sortField: 'id' }),
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
  searchSchema: Object,
  parseData: Function,
  hasPagination: {
    type: Boolean,
    default: true,
  },
  defaultVisibleSearchForm: Boolean,
  rowClick: Function,
  drag: Boolean,
  onDragEnd: Function,
  rowCalssName: Function,
  cellClassName: Function,
  cellStyle: Function,
  hideTag: Boolean,
  staticPage: Boolean,
});

const emit = defineEmits(['update:modelValue']);

const tableRef = ref(null);

const toolbarRef = ref();

const tableRadio = ref(null);

const eyeColumnKeys = ref([]);

const state = reactive({
  isLoading: false,
  list: [],
  total: 0,
  selected: [],
  prevPageId: 0,
});

const route = useRoute();

const currentParams = reactive(
  merge(props.api.data, {
    [props.filterKey]: {
      ...route.query,
    },
    pageRequest: {
      pageSize: 10,
      pageNo: 1,
      ...props.defaultSort,
      // lastID: 1,
    },
  }),
);

provide('currentParams', currentParams);

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
  const { url, method, dataPath = 'list' } = props.api;
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

  const { [dataPath]: list } = await request({
    url,
    method,
    data,
  });

  if (props.staticPage) {
    const { pageNo, pageSize } = currentParams.pageRequest;

    //使用静态分页
    state.list = list.slice((pageNo - 1) * pageSize, pageNo * pageSize);
    state.total = list.length;
  } else {
    if (Array.isArray(list)) {
      state.list = props.parseData ? props.parseData(list) : list;
    }
  }

  if (props.hasPagination && !props.staticPage) {
    const urlSplit = url.split('/');

    const totalUrlLast = urlSplit.at(-1).replace('search', 'total');

    const totalUrl = urlSplit.slice(0, -1).join('/') + `/${totalUrlLast} `;

    const { total } = await request({
      url: totalUrl,
      method,
      data,
    });

    if (total) {
      state.total = +total;
    }
  }

  state.isLoading = false;
  state.selected = [];
};

const debounceFetchData = debounce(fetchData);

watch(currentParams, (newVal, oldVal) => {
  debounceFetchData();
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
  if (order) {
    currentParams.pageRequest.sort = order.replace('ending', '').toUpperCase();
    currentParams.pageRequest.sortField = prop;
  } else {
    delete currentParams.pageRequest.sort;
    delete currentParams.pageRequest.sortField;
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

const initSort = () => {
  const el = tableRef.value.$el.querySelector('.el-table__body-wrapper tbody');
  Sortable.create(el, {
    animation: 200, //动画时长
    // handle: '.el-table__row', //可拖拽区域class
    draggable: '.el-table__row',
    // handle: '.is-drag', //可拖拽区域class
    filter: '.no-drag',
    //拖拽中事件
    onMove: ({ dragged, related }) => {
      const oldRow = state.list[dragged.rowIndex]; //旧位置数据
      const newRow = state.list[related.rowIndex]; //被拖拽的新数据
    },
    //拖拽结束事件
    onEnd: (evt) => {
      // if (evt.newIndex === 0) {
      //   state.list = props.dataSource;
      //   return;
      // }
      const curRow = state.list.splice(evt.oldIndex, 1)[0];

      state.list.splice(evt.newIndex, 0, curRow);

      props.onDragEnd && props.onDragEnd(state.list);
    },
  });
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
});

onMounted(() => {
  if (mode.value === 'static') {
    state.list = props.dataSource;
    state.total = props.dataSource.length;
  } else {
    fetchData();
  }

  eyeColumnKeys.value = props.columns
    .filter((item) => !item.hidden)
    .map((item) => item.title);

  if (props.drag) {
    initSort();
  }
});

//  抛出方法
defineExpose({ fetchData, currentParams, getData: () => state.list });
</script>
