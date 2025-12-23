import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';

export default (t) => ({
  items: [
    {
      label: t('task.name'),
      name: 'name',
      component: 'input',
    },
    {
      label: t('task.node'),
      name: 'nodeName',
      component: 'select',
      props: {
        mode: 'remote',
        api: nodeApi.getNodeList({ filters: {} }),
        labelKey: 'name',
        valueKey: 'name',
      },
    },
    {
      label: t('task.status'),
      name: 'status',
      component: 'select',
      props: {
        mode: 'static',
        options: [
          {
            label: t('task.statusCompleted'),
            value: 'closed',
          },
          {
            label: t('task.statusRunning'),
            value: 'success',
          },
          {
            label: t('task.statusFailed'),
            value: 'failed',
          },
          {
            label: t('task.statusUnknown'),
            value: 'unknown',
          },
        ],
      },
    },
    {
      label: t('task.card'),
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
});
