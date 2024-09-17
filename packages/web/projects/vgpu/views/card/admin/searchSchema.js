import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';

export default {
  items: [
    {
      label: '显卡 ID',
      name: 'uid',
      component: 'input',
    },
    {
      label: '所属节点',
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
      label: '显卡型号',
      name: 'type',
      component: 'select',
      props: {
        mode: 'remote',
        api: cardApi.getCardType(),
        labelKey: 'type',
        valueKey: 'type',
      },
    },
  ],
};
