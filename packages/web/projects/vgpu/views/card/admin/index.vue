<template>
  <list-header
    v-if="!hideTitle"
    :description="$t('card.description')"
  />

  <preview-bar
    v-if="!hideTitle"
    :title="$t('dashboard.card')"
    type="deviceuuid"
    :handle-click="handleClick"
    :handle-pie-click="handlePieClick"
    :currentName="currentType"
  />

  <table-plus
    :key="`${locale}-card-table`"
    :api="cardApi.getCardList({ filters })"
    :columns="columns"
    :rowAction="rowAction"
    :searchSchema="searchSchema"
    hideTag
    style="height: auto"
    ref="tableRef"
    staticPage
  >
  </table-plus>
</template>

<script setup lang="jsx">
import cardApi from '~/vgpu/api/card';
import { useRouter } from 'vue-router';
import createSearchSchema from '~/vgpu/views/card/admin/searchSchema';
import PreviewBar from '~/vgpu/components/previewBar.vue';
import { ref, watch, computed } from 'vue';
import { roundToDecimal } from '@/utils';
import { useI18n } from 'vue-i18n';

const props = defineProps(['hideTitle', 'filters']);

const router = useRouter();
const { t, locale } = useI18n();

const tableRef = ref(null);
const currentType = computed(() => tableRef.value?.currentParams?.filters?.type || '');
const searchSchema = computed(() => createSearchSchema(t));

const handleClick = (params) => {
  router.push({
    path: `/admin/vgpu/card/admin/${params.data.name}`,
  });
};

const columns = computed(() => [
  {
    title: t('card.id'),
    dataIndex: 'uuid',
    render: ({ uuid }) => (
      <text-plus text={uuid} to={`/admin/vgpu/card/admin/${uuid}`} />
    ),
  },
  {
    title: t('card.status'),
    dataIndex: 'health',
    render: ({ health, isExternal }) => (
        <el-tag disable-transitions type={isExternal ? 'warning' : (health ? 'success' : 'danger')}>
          {isExternal ? t('card.unmanaged') : (health ? t('card.healthy') : t('card.hardwareError'))}
        </el-tag>
    )
  },
  {
    title: t('card.mode'),
    dataIndex: 'mode',
    render: ({ mode, type }) => (
        <el-tag disable-transitions>
          {type?.split('-')[0] === "NVIDIA" ? mode : 'default'}
        </el-tag>
    )
  },
  {
    title: t('card.node'),
    dataIndex: 'nodeName',
  },
  {
    title: t('card.model'),
    dataIndex: 'type',
  },
  {
    title: t('card.vgpu'),
    dataIndex: 'used',
    render: ({ vgpuTotal, vgpuUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : vgpuUsed}/{isExternal ? '--' : vgpuTotal}
    </span>
    ),
  },
  {
    title: t('card.computeAllocTotal'),
    dataIndex: 'used',
    minWidth: 100,
    render: ({ coreTotal, coreUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : coreUsed}/{coreTotal}
    </span>
    ),
  },
  {
    title: t('card.memoryAllocTotal'),
    dataIndex: 'w',
    minWidth: 100,
    render: ({ memoryTotal, memoryUsed, isExternal }) => (
        <span>
      {isExternal ? '--' : roundToDecimal(memoryUsed / 1024, 1)}/
          {roundToDecimal(memoryTotal / 1024, 1)} GiB
    </span>
    ),
  },
]);

const rowAction = computed(() => [
  {
    title: t('card.viewDetails'),
    onClick: (row) => {
      router.push({
        path: `/admin/vgpu/card/admin/${row.uuid}`,
      });
    },
  },
]);

const PieRef = ref();

const handlePieClick = (params, echarts) => {
  PieRef.value = echarts;
  const name = params.data.name;
  if (tableRef.value?.currentParams?.filters?.type === name) {
    echarts.dispatchAction({
      type: 'downplay',
      seriesIndex: 0,
    });
    return delete tableRef.value.currentParams.filters.type;
  }

  echarts.dispatchAction({
    type: 'downplay',
    seriesIndex: 0,
  });

  echarts.dispatchAction({
    type: 'highlight',
    seriesIndex: 0,
    dataIndex: params.dataIndex,
  });
  tableRef.value.currentParams.filters.type = name;
};

watch(
  () => tableRef.value?.currentParams?.filters?.type,
  (newVal) => {
    if (!PieRef.value) return;
    const data = PieRef.value.getOption().series[0].data;
    if (newVal) {
      PieRef.value.dispatchAction({
        type: 'downplay',
        seriesIndex: 0,
      });

      PieRef.value.dispatchAction({
        type: 'highlight',
        seriesIndex: 0,
        dataIndex: data.findIndex((item) => item.name === newVal),
      });
    } else {
      PieRef.value.dispatchAction({
        type: 'downplay',
        seriesIndex: 0,
      });
    }
  },
);
</script>

<style></style>
