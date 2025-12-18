export default (Layout) => ({
  path: '/admin/vgpu',
  component: Layout,
  redirect: '/admin/vgpu/node/admin',
  name: 'vgpu',
  meta: {
    title: 'routes.gpuAdmin',
    icon: 'vgpu-gpu-l',
  },
  children: [
    {
      name: 'resource-admin',
      meta: { title: 'routes.resourceAdmin' },
      path: '/admin/vgpu/monitor',
      component: () => import('~/vgpu/views/monitor/index.vue'),
      children: [
        {
          path: '/admin/vgpu/monitor/overview',
          component: () => import('~/vgpu/views/monitor/overview/index.vue'),
          name: 'overview',
          meta: { title: 'routes.dashboard', icon: 'dashboard', noCache: true },
        },
        {
          path: '/admin/vgpu/node/admin',
          component: () => import('~/vgpu/views/node/admin/index.vue'),
          name: 'node-admin',
          meta: { title: 'routes.nodes', icon: 'vgpu-node', noCache: true },
        },
        {
          path: '/admin/vgpu/node/admin/:uid',
          component: () => import('~/vgpu/views/node/admin/Detail.vue'),
          name: 'node-admin-detail',
        },
        {
          path: '/admin/vgpu/card/admin',
          component: () => import('~/vgpu/views/card/admin/index.vue'),
          name: 'card-admin',
          meta: { title: 'routes.cards', icon: 'vgpu-card', noCache: true },
        },
        {
          path: '/admin/vgpu/card/admin/:uuid',
          component: () => import('~/vgpu/views/card/admin/Detail.vue'),
          name: 'card-admin-detail',
        },
        {
          path: '/admin/vgpu/task/admin',
          component: () => import('~/vgpu/views/task/admin/index.vue'),
          name: 'task-admin',
          meta: { title: 'routes.tasks', icon: 'vgpu-task', noCache: true },
        },
        {
          path: '/admin/vgpu/task/admin/detail',
          component: () => import('~/vgpu/views/task/admin/Detail.vue'),
          name: 'task-admin-detail',
        },
      ],
    },
  ],
});
