export const leftConfig = [
  {
    title: '资源状态',
    morePath: '/iac/manager/instance',
    list: [
      {
        title: '云主机分布',
        data: [
          { name: '华为云', value: 400 },
          { name: '阿里云', value: 300 },
          { name: '腾讯云', value: 100 },
        ],
        unit: '台',
      },
      {
        title: '云服务器状态',
        data: [
          { name: '开机', value: 700 },
          { name: '关机', value: 100 },
        ],
      },
      {
        title: '云硬盘状态',
        data: [
          { name: '使用中', value: 900 },
          { name: '闲置', value: 100 },
        ],
      },
      {
        title: '云网卡状态',
        data: [
          { name: '使用中', value: 300 },
          { name: '闲置', value: 200 },
        ],
      },
    ],
  },
  {
    title: '优化建议',
    morePath: '/iac/manager/instance',
    list: [
      { tip: '建议降配', value: 1 },
      { tip: '建议升配', value: 1 },
      { tip: '建议变更付费方式', value: 1 },
      { tip: '建议回收', value: 114 },
    ],
  },
  {
    title: '资源消耗',
    morePath: '/iac/manager/instance',
    list: [
      {
        title: '按主机数用量',
        data: [
          { name: '业务1', value: 400 },
          { name: '业务2', value: 300 },
          { name: '业务3', value: 100 },
        ],
        unit: '台',
      },
      {
        title: '按CPU用量',
        data: [
          { name: '业务1', value: 400 },
          { name: '业务2', value: 200 },
          { name: '业务3', value: 500 },
        ],
      },
      {
        title: '按内存用量',
        data: [
          { name: '业务1', value: 200 },
          { name: '业务2', value: 100 },
          { name: '业务3', value: 100 },
        ],
      },
      {
        title: '按存储数用量',
        data: [
          { name: '业务1', value: 400 },
          { name: '业务2', value: 600 },
          { name: '业务3', value: 300 },
        ],
      },
    ],
  },
];

export const rightConfig = {
  provides: [
    {
      title: 'CPU',
      percent: 20,
    },
    {
      title: '内存',
      percent: 67,
    },
    {
      title: '存储',
      percent: 50,
    },
  ],
  approves: [
    {
      title: '待审批',
      count: 80,
    },
    {
      title: '审批中',
      count: 15,
    },
    {
      title: '已完成',
      count: 228,
    },
  ],
  myAdmin: [
    {
      title: '云账号',
      count: 5,
    },
    {
      title: '云主机',
      count: 572,
    },
    {
      title: '云硬盘',
      count: 803,
    },
    {
      title: '宿主机',
      count: 12,
    },
    {
      title: '存储器',
      count: 31,
    },
  ],
};
