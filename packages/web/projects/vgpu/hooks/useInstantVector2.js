import cardApi from '~/vgpu/api/card';
import { onMounted, ref, watchEffect } from 'vue';

const useInstantVector = (configs, parseQuery = (query) => query) => {
  const data = ref(configs);

  const fetchData = async () => {
    const reqs = configs.map(({ query }, index) =>
      cardApi.getInstantVector({ query: parseQuery(query) }).then((res) => {
        const num = res.data.length ? res.data[0]?.value : 0;
        data.value[index] = { ...data.value[index], count: num, percent: num };
      }),
    );
    Promise.all(reqs);
  };

  // onMounted(fetchData);

  watchEffect(() => {
    fetchData();
  });

  return data;
};

export default useInstantVector;
