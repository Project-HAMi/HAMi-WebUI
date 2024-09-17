import { ref } from 'vue';
import request from '@/utils/request';

const useRequest = ({ url, method, params, data } = {}) => {
  const loading = ref(false);

  const run = async (options = {}) => {
    loading.value = true;

    try {
      const res = await request({
        url,
        method,
        params,
        data,
        ...options,
      });

      return res;
    } catch (e) {
      return Promise.reject(e);
    } finally {
      loading.value = false;
    }
  };

  return { loading, run };
};

export default useRequest;
