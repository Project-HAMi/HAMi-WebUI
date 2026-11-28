import { createRouter, createWebHistory } from 'vue-router';
import { getBasePath } from '@/utils/base-path.mjs';
/* Layout */
import Layout from '@/layout';

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
const legacyRoutes = [
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

/**
 * constantRoutes
 * a base page that does not have permission requirements
 * all roles can be accessed
 */

export const constantRoutes = [
  {
    path: '/',
    component: Layout,
    redirect: '/overview',
    children: [
      {
        path: '/overview',
        component: () => import('@/views/overview/index.vue'),
        name: 'overview',
        meta: { title: 'routes.dashboard', icon: 'dashboard', noCache: true },
      },
      {
        path: '/nodes',
        component: () => import('@/views/nodes/index.vue'),
        name: 'nodes',
        meta: { title: 'routes.nodes', icon: 'vgpu-node', noCache: true },
      },
      {
        path: '/nodes/:uid',
        component: () => import('@/views/nodes/Detail.vue'),
        name: 'node-detail',
      },
      {
        path: '/accelerators',
        component: () => import('@/views/accelerators/index.vue'),
        name: 'accelerators',
        meta: { title: 'routes.cards', icon: 'vgpu-card', noCache: true },
      },
      {
        path: '/accelerators/:uuid',
        component: () => import('@/views/accelerators/Detail.vue'),
        name: 'accelerator-detail',
      },
      {
        path: '/workloads',
        component: () => import('@/views/workloads/index.vue'),
        name: 'workloads',
        meta: { title: 'routes.tasks', icon: 'vgpu-task', noCache: true },
      },
      {
        path: '/workloads/:podUid/containers/:container',
        component: () => import('@/views/workloads/Detail.vue'),
        name: 'workload-detail',
      },
    ],
  },
  ...legacyRoutes,
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/error-page/404'),
    hidden: true,
  },
];

const router = createRouter({
  history: createWebHistory(getBasePath()),
  routes: constantRoutes,
});

// Older overview bookmarks used a separate physical-device list.
router.beforeEach((to) => {
  if (to.name === 'accelerators' && to.query.scope === 'physical') {
    return { path: to.path, query: { ...to.query, scope: undefined }, hash: to.hash, replace: true };
  }
});

export default router;
