import Mock from 'mockjs';

Mock.mock('/mock/resourceOverview', 'get', () => {
  return [
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
  ];
});

Mock.mock('/mock/resourceStatus', 'get', () => {
  return {
    instanceRunning: 52,
    instanceStopped: 16,
    instanceError: 4,
    volumeRunning: 766,
    volumeStopped: 37,
    nicRunning: 19,
    nicStopped: 12,
  };
});
