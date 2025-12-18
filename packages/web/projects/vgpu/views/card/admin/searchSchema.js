import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';

export default (t) => ({
  items: [
    {
      label: t('card.id'),
      name: 'uid',
      component: 'input',
    },
    {
      label: t('card.node'),
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
      label: t('card.model'),
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
});
