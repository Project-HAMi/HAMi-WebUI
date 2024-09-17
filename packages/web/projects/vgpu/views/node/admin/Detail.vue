<template>
  <div>
    <back-header>
      节点管理 > {{ detail.name }}
<!--      <template #extra>-->
<!--        <el-form-item-->
<!--          label="节点调度"-->
<!--          style="margin-bottom: 0; margin-right: 20px"-->
<!--        >-->
<!--          <el-radio-group-->
<!--            :disabled="detail.isExternal"-->
<!--            v-model="tempSchedulable"-->
<!--            size="small"-->
<!--            @change="onChangeSchedulable"-->
<!--          >-->
<!--            <el-radio-button label="启用" :value="true" />-->
<!--            <el-radio-button label="禁用" :value="false" />-->
<!--          </el-radio-group>-->
<!--        </el-form-item>-->
<!--      </template>-->
    </back-header>

    <block-box class="node-block">
      <div class="node-detail">
        <div class="node-detail-left">
          <div class="title">详细信息</div>
          <ul class="node-detail-info">
            <li v-for="{ label, value, render } in detailColumns" :key="label">
              <span class="label">{{ label }}</span>
              <component v-if="render" :is="render(detail)" />
              <span v-else class="value">{{ detail[value] }}</span>
            </li>
          </ul>
        </div>


      </div>
    </block-box>

    <block-box>
      <ul class="card-gauges">
        <li v-for="(item, index) in gaugeConfig" :key="index">
          <template v-if="!detail.isExternal || index >= 2">
            <Gauge v-bind="item" />
          </template>
          <template v-else-if="detail.isExternal && index < 2">
            <el-empty description="暂无资源分配数据" :image-size="90" />
          </template>
        </li>
      </ul>
    </block-box>

    <div class="line-box">
      <block-box title="资源分配趋势（%）">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus
              :options="
              getRangeOptions({
                core: gaugeConfig[0].data,
                memory: gaugeConfig[1].data,
              })
            "
          />
        </div>
      </block-box>
      <block-box title="资源使用趋势（%）">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus
            :options="
              getRangeOptions({
                core: gaugeConfig[2].data,
                memory: gaugeConfig[3].data,
              })
            "
          />
        </div>
      </block-box>
    </div>

    <block-box title="显卡列表">
      <CardList :hideTitle="true" :filters="{ nodeUid: detail.uid }" />
    </block-box>

    <block-box title="任务列表">
      <template v-if="detail.isExternal">
        <el-alert title="由于节点未纳管，无法获取到任务数据" show-icon type="warning" :closable="false" />
        <el-empty description="暂无任务数据" :image-size="100" />
      </template>
      <template v-else>
        <TaskList :hideTitle="true" :filters="{ nodeUid: detail.uid }" />
      </template>
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import { useRoute, useRouter } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import {computed, onMounted, ref, watch} from 'vue';
import { Tools } from '@element-plus/icons-vue';
import CardList from '~/vgpu/views/card/admin/index.vue';
import TaskList from '~/vgpu/views/task/admin/index.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import EchartsPlus from '@/components/Echarts-plus.vue';
import TimeSelect from '~/vgpu/components/timeSelect.vue';
import nodeApi from '~/vgpu/api/node';
import { getLineOptions } from '~/vgpu/views/monitor/overview/getOptions';
import { ElMessage, ElMessageBox } from 'element-plus';
import api from '~/vgpu/api/task';
import { getRangeOptions } from './getOptions';
import {getDaysInRange} from "@/utils";

const route = useRoute();
const router = useRouter();

const detail = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const isSchedulable = ref(true);
const tempSchedulable = ref(isSchedulable.value);

const cp = useInstantVector(
  [
    {
      label: 'vGPU 超配',
      count: '0',
      query: `avg(hami_vgpu_count{node=~"$node"})`,
    },
    {
      label: '算力超配',
      count: '0',
      query: `avg(hami_vcore_scaling{node=~"$node"})`,
    },
    {
      label: '显存超配',
      count: '1.5',
      query: `avg(hami_vmemory_scaling{node=~"$node"})`,
    },
  ],
  (query) => query.replaceAll('$node', detail.value.name),
);

const gaugeConfig = useInstantVector(
  [
    {
      title: '算力分配率',
      percent: 0,
      query: `avg(sum(hami_container_vcore_allocated{node=~"$node"}) by (instance))`,
      totalQuery: `avg(sum(hami_core_size{node=~"$node"}) by (instance))`,
      percentQuery: `avg(sum(hami_container_vcore_allocated{node=~"$node"}) by (instance)) / avg(sum(hami_core_size{node=~"$node"}) by (instance)) *100`,
      total: 0,
      used: 0,
      unit: ' ',
    },
    {
      title: '显存分配率',
      percent: 0,
      query: `avg(sum(hami_container_vmemory_allocated{node=~"$node"}) by (instance)) / 1024`,
      totalQuery: `avg(sum(hami_memory_size{node=~"$node"}) by (instance)) / 1024`,
      percentQuery: `(avg(sum(hami_container_vmemory_allocated{node=~"$node"}) by (instance)) / 1024) /(avg(sum(hami_memory_size{node=~"$node"}) by (instance)) / 1024) *100`,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
    {
      title: '算力使用率',
      percent: 0,
      query: `avg(sum(hami_core_util{node=~"$node"}) by (instance))`,
      percentQuery: `avg(sum(hami_core_util_avg{node=~"$node"}) by (instance))`,
      totalQuery: `avg(sum(hami_core_size{node=~"$node"}) by (instance))`,
      total: 100,
      used: 0,
      unit: ' ',
    },
    {
      title: '显存使用率',
      percent: 0,
      query: `avg(sum(hami_memory_used{node=~"$node"}) by (instance)) / 1024`,
      totalQuery: `avg(sum(hami_memory_size{node=~"$node"}) by (instance))/1024`,
      percentQuery: `(avg(sum(hami_memory_used{node=~"$node"}) by (instance)) / 1024)/(avg(sum(hami_memory_size{node=~"$node"}) by (instance))/1024)*100`,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
  ],
  (query) => query.replaceAll(`$node`, detail.value.name),
  times,
);

const detailColumns = [
  {
    label: '节点状态',
    value: 'status',
    render: ({ isSchedulable, isExternal }) => {
      if (detail.value && detail.value.isSchedulable !== undefined) {
        return (
            <el-tag disable-transitions type={isExternal ? 'warning' : (isSchedulable ? 'success' : 'danger')}>
              {isExternal ? '未纳管' : (isSchedulable ? '可调度' : '禁止调度')}
            </el-tag>
        );
      } else {
        return <el-tag disable-transitions size="small" type="info">加载中...</el-tag>;
      }
    },
  },
  {
    label: '节点 IP 地址',
    value: 'ip',
    render: ({ ip }) => <text-plus text={ip} copy />,
  },
  {
    label: '节点 UUID',
    value: 'uid',
    render: ({ uid }) => <text-plus text={uid} copy />,
  },
  {
    label: '操作系统类型',
    value: 'operatingSystem',
    render: ({ operatingSystem }) => (
        <span>
          {operatingSystem==='' ? '--' : operatingSystem}
        </span>
    ),
  },
  {
    label: '系统架构',
    value: 'architecture',
    render: ({ architecture }) => (
        <span>
          {architecture==='' ? '--' : architecture}
        </span>
    ),
  },
  {
    label: 'kubelet 版本',
    value: 'kubeletVersion',
    render: ({ kubeletVersion }) => (
        <span>
          {kubeletVersion==='' ? '--' : kubeletVersion}
        </span>
    ),
  },
  {
    label: '操作系统版本',
    value: 'osImage',
    render: ({ osImage }) => (
        <span>
          {osImage==='' ? '--' : osImage}
        </span>
    ),
  },
  {
    label: '内核版本',
    value: 'kernelVersion',
    render: ({ kernelVersion }) => (
        <span>
          {kernelVersion==='' ? '--' : kernelVersion}
        </span>
    ),
  },
  {
    label: 'kube-proxy 版本',
    value: 'kubeProxyVersion',
    render: ({ kubeProxyVersion }) => (
        <span>
          {kubeProxyVersion==='' ? '--' : kubeProxyVersion}
        </span>
    ),
  },
  {
    label: '容器运行时',
    value: 'containerRuntimeVersion',
    render: ({ containerRuntimeVersion }) => (
        <span>
          {containerRuntimeVersion==='' ? '--' : containerRuntimeVersion}
        </span>
    ),
  },
  {
    label: '显卡数量',
    value: 'cardCnt',
    render: ({ cardCnt }) => (
        <span>
          {cardCnt==='' ? '--' : cardCnt}
        </span>
    ),
  },
  {
    label: '创建时间',
    value: 'creationTimestamp',
    render: ({ creationTimestamp }) => (
        <span>
          {creationTimestamp==='' ? '--' : creationTimestamp}
        </span>
    ),
  },
];

const onChangeSchedulable = (val) => {
  ElMessageBox.confirm(
    `确认对该节点进行${val ? '启用' : '禁用'}操作？`,
    '操作确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    },
  )
    .then(async () => {
      try {
        await nodeApi
          .stop({
            nodeName: detail.value.name,
            switch: val ? 'off' : 'on',
          })
          .then(() => {
            setTimeout(() => {
              refresh();
              ElMessage.success(`${val ? '启用' : '禁用'}成功`);
            }, 500);
          });
      } catch (error) {
        ElMessage.error(error.message);
        tempSchedulable.value = isSchedulable.value;
      }
    })
    .catch(() => {
      tempSchedulable.value = isSchedulable.value;
    });
};

const refresh = async () => {
  detail.value = await nodeApi.getNodeDetail({ uid: route.params.uid });
  isSchedulable.value = detail.value.isSchedulable;
};

onMounted(async () => {
  await refresh();
});
</script>

<style lang="scss">
.node-detail {
  display: flex;
  height: 100%;
  //gap: 50px;

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .title {
    color: #1d2b3a;
    font-family: 'PingFang SC';
    font-size: 14px;
    font-style: normal;
    font-weight: 500;
    //line-height: 20px;
    margin-bottom: 20px;
  }
  //.node-detail-left {
  //  min-width: 800px;
  //}
  .node-detail-info {
    gap: 15px;
    font-size: 12px;
    display: grid;
    grid-template-columns: repeat(3, 1fr);

    .label {
      display: inline-block;
      width: 100px;
      height: 20px;
      color: #939ea9;
    }

    .set {
      :hover {
        cursor: pointer;
        color: #324558;
      }
    }

    .cp {
      display: flex;
      gap: 25px;
    }
  }

  .gauges {
    flex: 1;
    display: flex;
    li {
      flex: 1;
    }
  }
}

.card-gauges {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  height: 200px;
  li {
    flex: 1;
  }
}

.line-box {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: 20px;
}

.node-block {
  display: flex;
  flex-direction: column;
  .home-block-content {
    flex: 1;
  }
}
</style>
