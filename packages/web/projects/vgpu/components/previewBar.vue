<template>
  <ul class="preview">
    <li class="preview-item" style="width: 20%; flex: none" v-if="!hidePie">
      <block-box
        :title="`${type === 'node' ? '节点显卡厂商分布' : '显卡类型分布'}`"
        class="nodeCard"
      >
        <div class="pie">
          <echarts-plus
            :options="getPreviewBarPie(pieData, props)"
            :onClick="handlePieClick"
            ref="echartsRef"
          />
        </div>

        <ul class="nodeCard-legend">
          <li
            v-for="{ name, value, color } in pieData"
            :style="{
              fontWeight: currentName === name ? 'bold' : 'normal',
            }"
          >
            <div class="left">
              <span
                class="color-box"
                :style="{
                  'background-color': color,
                }"
              ></span>
              <span> {{ name }}</span>
            </div>

            <span>{{ value }}</span>
          </li>
        </ul>
      </block-box>
    </li>
    <li class="preview-item">
      <TabTop v-bind="totalTop" :onClick="handleClick" class="node-top" />
    </li>
    <li class="preview-item">
      <TabTop v-bind="usedTop" :onClick="handleClick" class="node-top" />
    </li>
  </ul>
</template>

<script setup>
import BlockBox from '@/components/BlockBox.vue';
import EchartsPlus from '@/components/Echarts-plus.vue';
import { getPreviewBarPie, getTopOptions } from '~/vgpu/components/config';
import { onMounted, reactive, ref } from 'vue';
import cardApi from '~/vgpu/api/card';
import TabTop from '~/vgpu/components/TabTop.vue';

const props = defineProps({
  title: {
    default: '节点',
  },
  type: {
    default: 'node',
  },
  hidePie: Boolean,
  handleClick: Function,
  handlePieClick: Function,
  currentName: String,
});

const echartsRef = ref();

const totalTop = {
  title: `${props.title}资源分配率 Top5`,
  config: [
    {
      tab: 'vGPU',
      key: 'vgpu',
      nameKey: props.type,
      data: [],
      query: `topk(5, sum(hami_container_vgpu_allocated{}) by (${props.type}) / sum(hami_vgpu_count{}) by (${props.type}) * 100)`,
    },
    {
      tab: '算力',
      key: 'core',
      nameKey: props.type,
      data: [],
      query: `topk(5, sum(hami_container_vcore_allocated{}) by (${props.type}) / sum(hami_core_size{}) by (${props.type}) * 100)`,
    },
    {
      tab: '显存',
      key: 'memory',
      data: [],
      nameKey: props.type,
      query: `topk(5, sum(hami_container_vmemory_allocated{}) by (${props.type}) / sum(hami_memory_size{}) by (${props.type}) * 100)`,
    },
  ],
};

const usedTop = {
  title: `${props.title}资源使用率 Top5`,
  config: [
    {
      tab: '算力',
      key: 'core',
      nameKey: props.type,
      data: [],
      query: `topk(5, avg(hami_core_util_avg) by (${props.type}))`,
    },
    {
      tab: '显存',
      key: 'memory',
      data: [],
      nameKey: props.type,
      query: `topk(5, sum(hami_memory_used) by (${props.type}) / sum(hami_memory_size) by (${props.type}) * 100)`,
    },
  ],
};

const pieConfig = {
  deviceuuid: {
    query:
      'count by (devicetype) (sum by (deviceuuid, devicetype) (hami_vgpu_count))',
    key: 'devicetype',
  },
  node: {
    query: 'count by (provider) (sum by (node,provider) (hami_vgpu_count))',
    key: 'provider',
  },
};

const pieData = ref([]);

onMounted(async () => {
  const thisPieConfig = pieConfig[props.type];

  const { data } = await cardApi.getInstantVector({
    query: thisPieConfig.query,
  });

  const colors = [
    '#5470C6',
    '#91CC75',
    '#FAC858',
    '#EE6666',
    '#73C0DE',
    '#3BA272',
    '#FC8452',
    '#9A60B4',
    '#EA7CCC',
  ];
  pieData.value = data.map((item, index) => {
    return {
      name: item.metric[thisPieConfig.key],
      value: item.value,
      color: colors[index],
    };
  });
});
</script>

<style scoped lang="scss">
ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
.preview {
  width: 100%;
  display: flex;
  gap: 20px;
  margin-bottom: 20px;
  .preview-item {
    flex: 1;
  }

  .nodeCard {
    height: 100%;

    .pie {
      width: 200px;
      height: 200px;
      margin: 0 auto;
    }

    .nodeCard-legend {
      width: 100%;
      display: flex;
      flex-direction: column;
      gap: 15px;
      max-height: calc(3 * (12px + 15px));
      overflow-y: auto;
      padding-right: 10px;

      /* 自定义滚动条样式（可选） */
      &::-webkit-scrollbar {
        width: 6px;
      }

      &::-webkit-scrollbar-thumb {
        background-color: rgba(0, 0, 0, 0.2);
        border-radius: 3px;
      }

      li {
        display: flex;
        justify-content: space-between;
        font-size: 12px;
        align-items: center;
        .left {
          display: flex;
          align-items: center;
          gap: 5px;
        }
        .color-box {
          width: 10px;
          height: 10px;
          display: inline-block;
        }
      }
    }
  }

  .node-top {
    display: flex;
    flex-direction: column;
    min-height: 350px;
    height: 100%;
    & > :nth-child(2) {
      flex: 1;
      max-height: 280px;
    }
    .node-top-echarts {
      //flex: 1;
    }
  }
}
</style>
