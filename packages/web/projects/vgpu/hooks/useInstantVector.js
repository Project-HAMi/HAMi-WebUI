import cardApi from '~/vgpu/api/card';
import { ref, watch, watchEffect } from 'vue';
import { timeParse, calculatePrometheusStep } from '@/utils';
import {
  calculateMetricPercent,
  METRIC_STATUS,
  readInstantValue,
  requiresMetricCapacity,
  resolveMetricStatus,
  setMetricErrorState,
} from './instant-vector-state.mjs';

const useInstantVector = (configs, parseQuery = (query) => query, times) => {
  const data = ref(
    configs.map((config) => ({
      ...config,
      status: METRIC_STATUS.LOADING,
    })),
  );

  const safeParseQuery = (q) => {
    try {
      const parsed = parseQuery(q);
      return typeof parsed === 'string' ? parsed : undefined;
    } catch {
      return undefined;
    }
  };

  const fetchInstantData = async () => {
    const reqs = configs.map(
      async (config, index) => {
        const { query, totalQuery, percentQuery } = config;
        const metric = data.value[index];
        const requiresCapacity = requiresMetricCapacity(config);
        metric.status = METRIC_STATUS.LOADING;
        const parsedQuery = query ? safeParseQuery(query) : undefined;
        if (!parsedQuery || parsedQuery.includes('undefined')) {
          return;
        }
        try {
          let usedStatus = METRIC_STATUS.MISSING;
          let totalStatus;
          if (query) {
            const usedData = await cardApi.getInstantVector({
              query: parsedQuery,
            });
            const usedSample = readInstantValue(usedData);

            // Preserve whether Prometheus returned a real sample. A zero sample
            // is valid; an empty vector means that the metric is unavailable.
            metric.hasData = usedSample.status === METRIC_STATUS.READY;
            metric.count = usedSample.value;
            metric.used = usedSample.value;
            usedStatus = usedSample.status;
          }

          if (totalQuery) {
            const parsedTotalQuery = safeParseQuery(totalQuery);
            if (!parsedTotalQuery || parsedTotalQuery.includes('undefined')) {
              return;
            }
            const totalData = await cardApi.getInstantVector({
              query: parsedTotalQuery,
            });
            const totalSample = readInstantValue(totalData);
            totalStatus = totalSample.status;
            metric.totalHasData = totalSample.status === METRIC_STATUS.READY;
            metric.total = totalSample.value;
          }

          metric.status = resolveMetricStatus({
            usedStatus,
            totalStatus,
            total: metric.total,
            requiresCapacity,
          });

          if (requiresCapacity) {
            const percent = calculateMetricPercent(metric.used, metric.total);
            metric.percent = percent ?? 0;
          }

          if (percentQuery && times?.value?.[0] && times?.value?.[1]) {
            const parsedPercentQuery = safeParseQuery(percentQuery);
            if (
              !parsedPercentQuery ||
              parsedPercentQuery.includes('undefined')
            ) {
              return;
            }
            try {
              const percentData = await cardApi.getRangeVector({
                query: parsedPercentQuery,
                range: {
                  start: timeParse(times.value[0]),
                  end: timeParse(times.value[1]),
                  step: calculatePrometheusStep(times.value[0], times.value[1]),
                },
              });

              metric.data = percentData.data[0]?.values || [];
            } catch {
              metric.data = [];
            }
          }
        } catch {
          setMetricErrorState(metric, {
            hasTotal: Boolean(totalQuery),
            requiresCapacity,
          });
        }
      },
    );

    await Promise.all(reqs);
  };

  const fetchRangeData = async () => {
    const reqs = configs.map(
      async ({ query, totalQuery, percentQuery }, index) => {
        const parsedQuery = query ? safeParseQuery(query) : undefined;
        if (!parsedQuery || parsedQuery.includes('undefined')) {
          return;
        }

        if (percentQuery && times?.value?.[0] && times?.value?.[1]) {
          const parsedPercentQuery = safeParseQuery(percentQuery);
          if (!parsedPercentQuery || parsedPercentQuery.includes('undefined')) {
            return;
          }
          const percentData = await cardApi.getRangeVector({
            query: parsedPercentQuery,
            range: {
                start: timeParse(times.value[0]),
                end: timeParse(times.value[1]),
              step: calculatePrometheusStep(times.value[0], times.value[1]),
            },
          });

          data.value[index].data = percentData.data[0]?.values || [];
        }
      },
    );

    await Promise.all(reqs);
  };

  watchEffect(() => {
    fetchInstantData();
  });

  if (times) {
    watch(
      () => times.value,
      () => {
        // Only fetch data when both start and end times are available
        if (times?.value?.[0] && times?.value?.[1]) {
          fetchRangeData();
        }
      },
    );
  }

  return data;
};

export default useInstantVector;
