export const getRangeConfigInit = (t) => [
  {
    title: t('dashboard.gpuComputeAllocUsageTrend'),
    dataSource: [
      {
        name: t('dashboard.allocRateLegend'),
        query: `sum(hami_container_vcore_allocated) / sum(hami_core_size) * 100`,
        data: [],
        type: 'line',
        itemStyle: {
          borderColor: 'rgb(84, 112, 198)',
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)',
        },
      },
      {
        name: t('dashboard.usageRateLegend'),
        query: `avg(hami_core_util_avg)`,
        data: [],
        itemStyle: {
          borderColor: 'rgb(145, 204, 117)',
        },
        lineStyle: {
          color: 'rgb(145, 204, 117)',
        },
      },
    ],
  },
  {
    title: t('dashboard.gpuMemAllocUsageTrend'),
    dataSource: [
      {
        name: t('dashboard.allocRateLegend'),
        query: `sum(hami_container_vmemory_allocated) / sum(hami_memory_size) * 100`,
        data: [],
        itemStyle: {
          borderColor: 'rgb(84, 112, 198)',
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)',
        },
      },
      {
        name: t('dashboard.usageRateLegend'),
        query: `sum(hami_memory_used) / sum(hami_memory_size) * 100`,
        data: [],
        itemStyle: {
          borderColor: 'rgb(145, 204, 117)',
        },
        lineStyle: {
          color: 'rgb(145, 204, 117)',
        },
      },
    ],
  },
];
