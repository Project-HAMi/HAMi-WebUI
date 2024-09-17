import { createRouter, createWebHistory } from 'vue-router';
import { pick, uniqBy } from 'lodash';
/* Layout */
import Layout from '@/layout';
/* Router Modules */
import vgpuRoutes from '../../projects/vgpu/router';
// import vgpuHomeRoutes from '../../projects/vgpu/homeRouter';
import { nextTick } from 'vue';

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
  {
    path: '/404',
    component: () => import('@/views/error-page/404'),
    hidden: true,
  },
  {
    path: '/401',
    component: () => import('@/views/error-page/401'),
    hidden: true,
  },
  {
    path: '/',
    redirect: '/admin/vgpu/monitor/overview',
    component: Layout,
    hidden: true,
  },
  vgpuRoutes(Layout),
];

export const asyncRoutes = [
  // vgpuHomeRoutes(Layout),
  // vgpuRoutes(Layout),
];

const router = createRouter({
  history: createWebHistory(),
  routes: constantRoutes,
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

  nextTick(() => {
    const mainEl = document.getElementById('content');
    try {
      mainEl.scrollTo(0, 0);
    } catch (error) {
    }
  });
});

export default router;
