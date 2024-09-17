import api from '~/vgpu/api/card';

export default {
  items: [
    {
      label: 'IP',
      name: 'ip',
      component: 'input',
    },
    {
      label: '节点状态',
      name: 'isSchedulable',
      component: 'select',
      props: {
        mode: 'static',
        options: [
          {
            label: '可调度',
            value: 'true',
          },
          {
            label: '禁止调度',
            value: 'false',
          },
        ],
      },
    },
    {
      label: '显卡型号',
      name: 'type',
      component: 'select',
      props: {
        mode: 'remote',
        api: api.getCardType(),
        labelKey: 'type',
        valueKey: 'type',
      },
    },
  ],
};
