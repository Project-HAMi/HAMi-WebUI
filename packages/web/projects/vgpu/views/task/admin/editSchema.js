import api from '~/vgpu/api/card';

export default {
  labelWidth: '150px',
  labelAlign: 'top',
  items: [
    {
      label: '任务名称',
      name: 'name',
      component: 'input',
      required: true,
    },
    {
      label: '优先级',
      name: 'priority',
      component: 'radio',
      props: {
        optionType: 'button',
        mode: 'static',
        options: [
          {
            label: '低优先级',
            value: '0',
          },

          {
            label: '高优先级',
            value: '2',
          },
        ],
      },
      initialValue: '0',
    },

    {
      label: '实例数',
      name: 'replicas',
      component: 'inputNumber',
      required: true,
      initialValue: 1,
    },
    {
      label: '显卡类型',
      name: 'gpuType',
      component: 'select',
      props: {
        mode: 'remote',
        api: api.getCardList(),
        labelKey: 'type',
        valueKey: 'type',
      },
    },

    {
      label: 'GPU申请数',
      name: 'gpuCount',
      component: 'inputNumber',
      required: true,
      initialValue: 1,
      props: {
        unit: '个',
      },
    },

    {
      label: '显存申请数',
      name: 'gpuMem',
      component: 'inputNumber',
      props: {
        unit: 'M',
      },
    },
    {
      label: '算力',
      name: 'gpuCore',
      component: 'inputNumber',
      props: {
        unit: '%',
      },
    },
  ],
};
