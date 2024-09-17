import cardApi from '~/vgpu/api/card';
import { ref, watchEffect } from 'vue';
import { timeParse } from '@/utils';

const useInstantVector = (configs, parseQuery = (query) => query) => {
  const data = ref(configs);

  const fetchData = async () => {
    const start = new Date();
    start.setTime(start.getTime() - 3600 * 1000 * 24 * 7);

    const reqs = configs.map(({ query }) =>
      cardApi.getRangeVector({
        range: {
          start: timeParse(start),
          end: timeParse(new Date()),
          step: '1m',
        },
        query: item.query.replace('$node', detail.value.name),
      }),
    );
    const res = await Promise.all(reqs);
    data.value = data.value.map((item, i) => {
      const num = res[i].data.length ? res[i].data[0]?.value : 0;
      return { ...item, count: num, percent: num };
    });
  };

  // onMounted(fetchData);

  watchEffect(() => {
    fetchData();
  });

  return data;
};

export default useInstantVector;
