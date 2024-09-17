import axios from 'axios';

import { ElMessage, ElMessageBox, ElNotification } from 'element-plus';

const service = axios.create({
  baseURL: process.env.VUE_APP_BASE_API, // url = base url + request url
  timeout: 5000,
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
    if (res.code != null && res.code > 0 && res.code != 200) {
      // 50008: Illegal token; 50012: Other clients logged in; 50014: Token expired;
      if (res.code === 50008 || res.code === 50012 || res.code === 50014) {
        // to re-login
        await ElMessageBox.alert('请求异常', '提示');
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
