import cardApi from '~/vgpu/api/card';
import { onMounted, ref, watch, watchEffect } from 'vue';
import { timeParse } from '@/utils';

const useInstantVector = (configs, parseQuery = (query) => query, times) => {
  const data = ref(configs);

  const fetchInstantData = async () => {
    const reqs = configs.map(
      async ({ query, totalQuery, percentQuery, cntQuery }, index) => {
        if (parseQuery(query).includes('undefined')) {
          return;
        }
        if (query) {
          const usedData = await cardApi.getInstantVector({
            query: parseQuery(query),
          });

          const used = usedData.data.length ? usedData.data[0]?.value : 0;
          data.value[index].count = used;
          data.value[index].used = used;
        }

        if (totalQuery) {
          const totalData = await cardApi.getInstantVector({
            query: parseQuery(totalQuery),
          });
          if (totalData.data[0]) {
            data.value[index].total = totalData.data[0].value;
          }
        }
        if (data.value[index].total !== 0) {
            data.value[index].percent = data.value[index].used / data.value[index].total * 100;
        }
        if (percentQuery) {
          const percentData = await cardApi.getRangeVector({
            query: parseQuery(percentQuery),
            range: {
                start: timeParse(times?.value[0]),
                end: timeParse(times?.value[1]),
              step: '1m',
            },
          });

          data.value[index].data = percentData.data[0]?.values;
        }
      },
    );

    Promise.all(reqs);
  };

  const fetchRangeData = async () => {
    const reqs = configs.map(
      async ({ query, totalQuery, percentQuery }, index) => {
        if (parseQuery(query).includes('undefined')) {
          return;
        }

        if (percentQuery) {
          const percentData = await cardApi.getRangeVector({
            query: parseQuery(percentQuery),
            range: {
                start: timeParse(times.value[0]),
                end: timeParse(times.value[1]),
              step: '1m',
            },
          });

          data.value[index].data = percentData.data[0]?.values;
        }
      },
    );

    Promise.all(reqs);
  };

  watchEffect(() => {
    fetchInstantData();
  });

  watch(
    times,
    () => {
      fetchRangeData();
    },
    // { immediate: true },
  );

  return data;
};

export default useInstantVector;
