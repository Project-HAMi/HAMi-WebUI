import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';

export default {
  items: [
    {
      label: '任务名称',
      name: 'name',
      component: 'input',
    },
    {
      label: '节点名称',
      name: 'nodeName',
      component: 'select',
      props: {
        mode: 'remote',
        api: nodeApi.getNodeList({filters:{}}),
        labelKey: 'name',
        valueKey: 'name',
      },
    },
    {
      label: '任务状态',
      name: 'status',
      component: 'select',
      props: {
        mode: 'static',
        options: [
          {
            label: '已完成',
            value: 'closed',
          },
          {
            label: '运行中',
            value: 'success',
          },
          {
            label: '错误',
            value: 'failed',
          },
          {
            label: '未知',
            value: 'unknown',
          },
        ],
      },
    },
    {
      label: '显卡ID',
      name: 'deviceId',
      component: 'select',
      props: {
        mode: 'remote',
        api: cardApi.getCardList(),
        labelKey: 'uuid',
        valueKey: 'uuid',
      },
    },
  ],
};
