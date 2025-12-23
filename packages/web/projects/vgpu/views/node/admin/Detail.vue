<template>
  <div>
    <back-header>
      {{ $t('node.detail.title') }} > {{ detail.name }}
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
          <div class="title">{{ $t('node.detail.detailInfo') }}</div>
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
            <el-empty :description="$t('node.detail.noAllocData')" :image-size="90" />
          </template>
        </li>
      </ul>
    </block-box>

    <div class="line-box">
      <block-box :title="$t('node.detail.resourceAllocTrend')">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus
              :options="
              getRangeOptions({
                core: gaugeConfig[0].data,
                memory: gaugeConfig[1].data,
              }, t)
            "
          />
        </div>
      </block-box>
      <block-box :title="$t('node.detail.resourceUsageTrend')">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <div style="height: 200px">
          <echarts-plus
            :options="
              getRangeOptions({
                core: gaugeConfig[2].data,
                memory: gaugeConfig[3].data,
              }, t)
            "
          />
        </div>
      </block-box>
    </div>

    <block-box :title="$t('node.detail.cardList')">
      <CardList :key="locale" :hideTitle="true" :filters="{ nodeUid: detail.uid }" />
    </block-box>

    <block-box :title="$t('node.detail.taskList')">
      <template v-if="detail.isExternal">
        <el-alert :title="$t('node.detail.unmanagedNoTask')" show-icon type="warning" :closable="false" />
        <el-empty :description="$t('node.detail.noTaskData')" :image-size="100" />
      </template>
      <template v-else>
        <TaskList :key="locale" :hideTitle="true" :filters="{ nodeUid: detail.uid }" />
      </template>
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import { useRoute, useRouter } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { computed, onMounted, ref, watch } from 'vue';
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
import { useI18n } from 'vue-i18n';

const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const detail = ref({});

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const isSchedulable = ref(true);
const tempSchedulable = ref(isSchedulable.value);

const _cpConfig = [
  {
    labelKey: 'node.detail.vgpuOvercommit',
    count: '0',
    query: `avg(hami_vgpu_count{node=~"$node"})`,
  },
  {
    labelKey: 'node.detail.computeOvercommit',
    count: '0',
    query: `avg(hami_vcore_scaling{node=~"$node"})`,
  },
  {
    labelKey: 'node.detail.memoryOvercommit',
    count: '1.5',
    query: `avg(hami_vmemory_scaling{node=~"$node"})`,
  },
];

const cp = useInstantVector(
  _cpConfig.map(item => ({...item, label: t(item.labelKey)})),
  (query) => query.replaceAll('$node', detail.value.name),
);

const _gaugeConfigBase = [
  {
    titleKey: 'dashboard.computeAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vcore_allocated{node=~"$node"}) by (instance))`,
    totalQuery: `avg(sum(hami_core_size{node=~"$node"}) by (instance))`,
    percentQuery: `avg(sum(hami_container_vcore_allocated{node=~"$node"}) by (instance)) / avg(sum(hami_core_size{node=~"$node"}) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vmemory_allocated{node=~"$node"}) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size{node=~"$node"}) by (instance)) / 1024`,
    percentQuery: `(avg(sum(hami_container_vmemory_allocated{node=~"$node"}) by (instance)) / 1024) /(avg(sum(hami_memory_size{node=~"$node"}) by (instance)) / 1024) *100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
  {
    titleKey: 'dashboard.computeUsageRate',
    percent: 0,
    query: `avg(sum(hami_core_util{node=~"$node"}) by (instance))`,
    percentQuery: `avg(sum(hami_core_util_avg{node=~"$node"}) by (instance))`,
    totalQuery: `avg(sum(hami_core_size{node=~"$node"}) by (instance))`,
    total: 100,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memUsageRate',
    percent: 0,
    query: `avg(sum(hami_memory_used{node=~"$node"}) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size{node=~"$node"}) by (instance))/1024`,
    percentQuery: `(avg(sum(hami_memory_used{node=~"$node"}) by (instance)) / 1024)/(avg(sum(hami_memory_size{node=~"$node"}) by (instance))/1024)*100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
];

const gaugeConfig = useInstantVector(
  _gaugeConfigBase.map(item => ({...item, title: t(item.titleKey)})),
  (query) => query.replaceAll(`$node`, detail.value.name),
  times,
);

const detailColumns = computed(() => [
  {
    label: t('node.status'),
    value: 'status',
    render: ({ isSchedulable, isExternal }) => {
      if (detail.value && detail.value.isSchedulable !== undefined) {
        return (
            <el-tag disable-transitions type={isExternal ? 'warning' : (isSchedulable ? 'success' : 'danger')}>
              {isExternal ? t('node.unmanaged') : (isSchedulable ? t('dashboard.schedulable') : t('dashboard.unschedulable'))}
            </el-tag>
        );
      } else {
        return <el-tag disable-transitions size="small" type="info">{t('node.detail.loading')}</el-tag>;
      }
    },
  },
  {
    label: t('node.detail.nodeIpAddress'),
    value: 'ip',
    render: ({ ip }) => <text-plus text={ip} copy />,
  },
  {
    label: t('node.detail.nodeUuid'),
    value: 'uid',
    render: ({ uid }) => <text-plus text={uid} copy />,
  },
  {
    label: t('node.detail.osType'),
    value: 'operatingSystem',
    render: ({ operatingSystem }) => (
        <span>
          {operatingSystem==='' ? '--' : operatingSystem}
        </span>
    ),
  },
  {
    label: t('node.detail.architecture'),
    value: 'architecture',
    render: ({ architecture }) => (
        <span>
          {architecture==='' ? '--' : architecture}
        </span>
    ),
  },
  {
    label: t('node.detail.kubeletVersion'),
    value: 'kubeletVersion',
    render: ({ kubeletVersion }) => (
        <span>
          {kubeletVersion==='' ? '--' : kubeletVersion}
        </span>
    ),
  },
  {
    label: t('node.detail.osVersion'),
    value: 'osImage',
    render: ({ osImage }) => (
        <span>
          {osImage==='' ? '--' : osImage}
        </span>
    ),
  },
  {
    label: t('node.detail.kernelVersion'),
    value: 'kernelVersion',
    render: ({ kernelVersion }) => (
        <span>
          {kernelVersion==='' ? '--' : kernelVersion}
        </span>
    ),
  },
  {
    label: t('node.detail.kubeProxyVersion'),
    value: 'kubeProxyVersion',
    render: ({ kubeProxyVersion }) => (
        <span>
          {kubeProxyVersion==='' ? '--' : kubeProxyVersion}
        </span>
    ),
  },
  {
    label: t('node.detail.containerRuntime'),
    value: 'containerRuntimeVersion',
    render: ({ containerRuntimeVersion }) => (
        <span>
          {containerRuntimeVersion==='' ? '--' : containerRuntimeVersion}
        </span>
    ),
  },
  {
    label: t('node.cardCount'),
    value: 'cardCnt',
    render: ({ cardCnt }) => (
        <span>
          {cardCnt==='' ? '--' : cardCnt}
        </span>
    ),
  },
  {
    label: t('node.detail.creationTime'),
    value: 'creationTimestamp',
    render: ({ creationTimestamp }) => (
        <span>
          {creationTimestamp==='' ? '--' : creationTimestamp}
        </span>
    ),
  },
]);

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
