import axios from 'axios';

import { ElMessage, ElMessageBox, ElNotification } from 'element-plus';
import i18n from '@/locales';
import { getBasePath } from '@/utils/base-path';

// Default request timeout in ms. Override at build time via VUE_APP_REQUEST_TIMEOUT
// (injected through .env.* or chart values.frontend.requestTimeout). 60s is large
// enough for the slowest known page-side API (/v1/nodes can take a few seconds
// against a large VictoriaMetrics cluster) while still bounding hung requests.
const DEFAULT_REQUEST_TIMEOUT = 60000;
const requestTimeout =
  Number.parseInt(process.env.VUE_APP_REQUEST_TIMEOUT, 10) || DEFAULT_REQUEST_TIMEOUT;

// API request URLs are root-absolute ("/api/vgpu/..."). Using the runtime base
// path as the axios baseURL makes them resolve under the sub-path (e.g.
// "/gpu-ui/api/vgpu/...") without depending on the injected <base> tag. At the
// site root — and in the Vite dev server, where window.__BASE_PATH__ is not
// injected — getBasePath() returns "/", giving the historical "/api/vgpu/..."
// exactly (superseding the build-time VUE_APP_BASE_API).
const service = axios.create({
  baseURL: getBasePath(), // url = base url + request url
  timeout: requestTimeout,
  validateStatus: function (status) {
    return (status >= 200 && status < 300) || status > 520;
  },
});

// request interceptor
service.interceptors.request.use(
  (config) => {
    return config;
  },
  (error) => {
    // do something with request error
    return Promise.reject(error);
  },
);

// response interceptor
service.interceptors.response.use(
  /**
   * If you want to get http information such as headers or status
   * Please return  response => response
   */

  /**
   * Determine the request status by custom code
   * Here is just an example
   * You can also judge the status by HTTP Status Code
   */
  async (response) => {
    const res = response.data;
    // if the custom code is not 0, it is judged as an error.
    if (res.code !== null && res.code > 0 && res.code !== 200) {
      // 50008: Illegal token; 50012: Other clients logged in; 50014: Token expired;
      if (res.code === 50008 || res.code === 50012 || res.code === 50014) {
        // to re-login
        await ElMessageBox.alert(i18n.global.t('common.requestError'), i18n.global.t('common.tip'));
      } else {
        ElNotification({
          title: res.reason,
          message: res.message,
          type: 'error',
          position: 'bottom-right',
        });
      }
      return Promise.reject(new Error(res.message || 'Error'));
    } else {
      return res;
    }
  },
  (error) => {
    ElMessage({
      message: error.message,
      type: 'error',
    });
    return Promise.reject(error);
  },
);

export default service;
