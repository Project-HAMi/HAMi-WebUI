import api from '~/vgpu/api/card';

// schema 需要根据当前语言动态渲染，导出函数传入 t
export default (t) => ({
  items: [
    {
      label: 'IP',
      name: 'ip',
      component: 'input',
    },
    {
      label: t('node.status'),
      name: 'isSchedulable',
      component: 'select',
      props: {
        mode: 'static',
        options: [
          {
            label: t('dashboard.schedulable'),
            value: 'true',
          },
          {
            label: t('dashboard.unschedulable'),
            value: 'false',
          },
        ],
      },
    },
    {
      label: t('node.cardModel'),
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
});
