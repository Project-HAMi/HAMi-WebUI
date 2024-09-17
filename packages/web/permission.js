import router from '@/router';
import NProgress from 'nprogress'; // progress bar
import 'nprogress/nprogress.css'; // progress bar style

/**
 * 登录鉴权：路由前置守卫
 */
// 白名单
const whiteList = [
  '/401',
  '/404',
];

router.beforeEach(async (to, from, next) => {
  NProgress.start();
  next();
});

router.afterEach(() => {
  // finish progress bar
  NProgress.done();
});
