import { createRouter, createWebHistory } from 'vue-router';
import { pick, uniqBy } from 'lodash';
import { getBasePath } from '@/utils/base-path.mjs';
/* Layout */
import Layout from '@/layout';
/* Router Modules */
import vgpuRoutes, { legacyRoutes } from '../../projects/vgpu/router';
// import vgpuHomeRoutes from '../../projects/vgpu/homeRouter';

/**
 * constantRoutes
 * a base page that does not have permission requirements
 * all roles can be accessed
 */

export const constantRoutes = [
  {
    path: '/redirect',
    component: Layout,
    hidden: true,
    children: [
      {
        path: '/redirect/:path(.*)',
        component: () => import('@/views/redirect/index'),
      },
    ],
  },
  vgpuRoutes(Layout),
  ...legacyRoutes,
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/error-page/404'),
    hidden: true,
  },
];

export const asyncRoutes = [
  // vgpuHomeRoutes(Layout),
  // vgpuRoutes(Layout),
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

router.afterEach((to) => {
  const history = JSON.parse(localStorage.getItem('history')) || [];
  if (to.path !== '/home') {
    history.unshift(to);
  }
  const newData = uniqBy(history, 'path')
    .slice(0, 7)
    .map((item) => pick(item, ['path', 'meta']));

  localStorage.setItem('history', JSON.stringify(newData));
});

export default router;
