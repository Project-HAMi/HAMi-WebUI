<template>
  <block-box :title="title">
    <template #extra>
      <el-radio-group v-model="tabActive" size="small" @change="handleTabChange">
        <el-radio-button
          v-for="{ tab, key } in config"
          :label="tab"
          :value="key"
        />
      </el-radio-group>
    </template>

    <echarts-plus
      :options="
        getTopOptions(
          currentConfig.find((item) => item.key === tabActive)?.data || [],
        )
      "
      :onClick="handleClick"
      style="min-height: 250px; height: 100%"
    />
  </block-box>
</template>

<script setup>
import BlockBox from '@/components/BlockBox.vue';
import { onMounted, ref } from 'vue';
import EchartsPlus from '@/components/Echarts-plus.vue';
import cardApi from '~/vgpu/api/card';
import { cloneDeep } from 'lodash';

const props = defineProps({
  title: String,
  key: String,
  config: Array,
  onClick: Function,
});
const currentConfig = ref(props.config);
const tabActive = ref('');

const handleTabChange = (key) => {
  tabActive.value = key;
};

const handleClick = (params) => {
  if (props.onClick) {
    props.onClick({ ...params, tabActive: tabActive.value });
  }
};

const getTopOptions = () => {
  const config = currentConfig.value.find(
    (item) => item.key === tabActive.value,
  );
  const data = cloneDeep(config?.data) || [];
  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'shadow',
      },
      formatter: function (params) {
        var res = params[0].name + '<br/>';
        for (var i = 0; i < params.length; i++) {
          res +=
            params[i].marker +
            params[i].seriesName +
            (+params[i].value).toFixed(0) +
            `${config.unit || '%'}<br/>`;
        }
        return res;
      },
    },
    legend: {
      show: false,
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      top: '0',
      containLabel: true,
    },
    xAxis: {
      type: 'value',
      boundaryGap: [0, 0.01],
    },
    yAxis: {
      type: 'category',
      data: data
        .reverse()
        .map((item) =>
          item.name.length > 15 ? item.name.slice(0, 15) + '...' : item.name,
        ),
    },
    series: [
      {
        name: '',
        type: 'bar',
        data: data,
      },
    ],
  };
};

onMounted(async () => {
  tabActive.value = currentConfig.value[0].key;
  currentConfig.value.forEach((v, i) => {
    cardApi
      .getInstantVector({
        query: v.query,
      })
      .then((res) => {
        currentConfig.value[i].data = res.data.map((item) => ({
          name: item.metric[v.nameKey],
          value: item.value,
        }));
      });
  });
});
</script>
