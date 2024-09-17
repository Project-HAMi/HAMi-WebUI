export const rangeConfigInit = [
  {
    title: '资源分配趋势',
    dataSource: [
      {
        name: 'vGPU',
        query: `sum(hami_container_vgpu_allocated) / sum(hami_vgpu_count) * 100`,
        data: [],
        type: 'line',
        areaStyle: {
          normal: {
            color: {
              type: 'linear',
              x: 0, // 渐变起始点 0%
              y: 0, // 渐变起始点 0%
              x2: 0, // 渐变结束点 100%
              y2: 1, // 渐变结束点 100%
              colorStops: [
                {
                  offset: 0,
                  color: 'rgba(250, 200, 88, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(250, 200, 88, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
        itemStyle: {
          color: 'rgb(250, 200, 88)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(250, 200, 88)', // 设置线条颜色为橙色
        },
      },
      {
        name: '算力',
        query: `sum(hami_container_vcore_allocated) / sum(hami_core_size) * 100`,
        data: [],
        type: 'line',
        areaStyle: {
          normal: {
            color: {
              type: 'linear',
              x: 0, // 渐变起始点 0%
              y: 0, // 渐变起始点 0%
              x2: 0, // 渐变结束点 100%
              y2: 1, // 渐变结束点 100%
              colorStops: [
                {
                  offset: 0,
                  color: 'rgba(84, 112, 198, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(84, 112, 198, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
        itemStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
      },
      {
        name: '显存',
        query: `sum(hami_container_vmemory_allocated) / sum(hami_memory_size) * 100`,
        data: [],
        areaStyle: {
          normal: {
            color: {
              type: 'linear',
              x: 0, // 渐变起始点 0%
              y: 0, // 渐变起始点 0%
              x2: 0, // 渐变结束点 100%
              y2: 1, // 渐变结束点 100%
              colorStops: [
                {
                  offset: 0,
                  color: 'rgba(34, 139, 34, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(34, 139, 34, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
        itemStyle: {
          color: 'rgb(145, 204, 117)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(145, 204, 117)', // 设置线条颜色为橙色
        },
      },
    ],
  },
  {
    title: '资源使用趋势',
    dataSource: [
      {
        name: '算力',
        query: `avg(hami_core_util_avg)`,
        data: [],
        areaStyle: {
          normal: {
            color: {
              type: 'linear',
              x: 0, // 渐变起始点 0%
              y: 0, // 渐变起始点 0%
              x2: 0, // 渐变结束点 100%
              y2: 1, // 渐变结束点 100%
              colorStops: [
                {
                  offset: 0,
                  color: 'rgba(84, 112, 198, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(84, 112, 198, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
        itemStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
      },
      {
        name: '显存',
        query: `sum(hami_memory_used) / sum(hami_memory_size) * 100`,
        data: [],
        areaStyle: {
          normal: {
            color: {
              type: 'linear',
              x: 0, // 渐变起始点 0%
              y: 0, // 渐变起始点 0%
              x2: 0, // 渐变结束点 100%
              y2: 1, // 渐变结束点 100%
              colorStops: [
                {
                  offset: 0,
                  color: 'rgba(34, 139, 34, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(34, 139, 34, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
        itemStyle: {
          color: 'rgb(145, 204, 117)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(145, 204, 117)', // 设置线条颜色为橙色
        },
      },
    ],
  },
];
