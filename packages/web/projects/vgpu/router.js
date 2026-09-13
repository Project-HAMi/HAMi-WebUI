export default (Layout) => ({
  path: '/',
  component: Layout,
  redirect: '/overview',
  children: [
    {
      path: '/overview',
      component: () => import('~/vgpu/views/monitor/overview/index.vue'),
      name: 'overview',
      meta: { title: 'routes.dashboard', icon: 'dashboard', noCache: true },
    },
    {
      path: '/nodes',
      component: () => import('~/vgpu/views/node/admin/index.vue'),
      name: 'nodes',
      meta: { title: 'routes.nodes', icon: 'vgpu-node', noCache: true },
    },
    {
      path: '/nodes/:uid',
      component: () => import('~/vgpu/views/node/admin/Detail.vue'),
      name: 'node-detail',
    },
    {
      path: '/accelerators',
      component: () => import('~/vgpu/views/card/admin/index.vue'),
      name: 'accelerators',
      meta: { title: 'routes.cards', icon: 'vgpu-card', noCache: true },
    },
    {
      path: '/accelerators/:uuid',
      component: () => import('~/vgpu/views/card/admin/Detail.vue'),
      name: 'accelerator-detail',
    },
    {
      path: '/workloads',
      component: () => import('~/vgpu/views/task/admin/index.vue'),
      name: 'workloads',
      meta: { title: 'routes.tasks', icon: 'vgpu-task', noCache: true },
    },
    {
      path: '/workloads/:podUid/containers/:container',
      component: () => import('~/vgpu/views/task/admin/Detail.vue'),
      name: 'workload-detail',
    },
  ],
});

const firstValue = (value) => (Array.isArray(value) ? value[0] : value);

const legacyWorkloadDetail = (to) => {
  const { name, podUid, ...query } = to.query;
  const container = firstValue(name);
  const uid = firstValue(podUid);
  return container && uid
    ? { name: 'workload-detail', params: { podUid: uid, container }, query }
    : { name: 'workloads', query };
};

// Earlier paths, kept so existing bookmarks and embeds still resolve.
export const legacyRoutes = [
  { path: '/admin/vgpu', redirect: '/nodes' },
  { path: '/admin/vgpu/monitor', redirect: '/overview' },
  { path: '/admin/vgpu/monitor/overview', redirect: '/overview' },
  { path: '/admin/vgpu/node/admin', redirect: '/nodes' },
  {
    path: '/admin/vgpu/node/admin/:uid',
    redirect: (to) => ({ name: 'node-detail', params: { uid: to.params.uid } }),
  },
  { path: '/admin/vgpu/card/admin', redirect: '/accelerators' },
  {
    path: '/admin/vgpu/card/admin/:uuid',
    redirect: (to) => ({ name: 'accelerator-detail', params: { uuid: to.params.uuid } }),
  },
  { path: '/admin/vgpu/task/admin', redirect: '/workloads' },
  { path: '/admin/vgpu/task/admin/detail', redirect: legacyWorkloadDetail },
];
