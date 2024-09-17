import Mock from './common';

Mock.mock('/mock/nodeList', 'post', ({ body }) => {
  return Mock.mock({
    status: 200,
    'list|1-10': [
      {
        group: `@first`,
        ip: '@ip',
        status: '@status',
        model: '@first',
        total: '@random',
      },
    ],
  });
});

Mock.mock('/mock/permit', 'post', ({ body }) => {
  return Mock.mock({
    status: 200,
    'list|1-10': [
      {
        uuid: `GPU-@first`,
        nodeName: 'node1',
        time: '@time',
        status: '@statusPermit',
        type: 'Tesla A100-PCIE-40GB',
      },
    ],
  });
});
