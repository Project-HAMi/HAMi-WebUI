import cardApi from '~/vgpu/api/card';
import { computed, ref, watch } from 'vue';
import { timeParse, calculatePrometheusStep } from '@/utils';
import {
  calculateMetricPercent,
  classifyParsedQuery,
  METRIC_STATUS,
  PARSED_QUERY_STATUS,
  readInstantValue,
  requiresMetricCapacity,
  resetMetricQueryState,
  restoreMetricState,
  resolveMetricStatus,
  setMetricErrorState,
  snapshotMetricState,
} from './instant-vector-state.mjs';
import {
  createRequestState,
  isLatestRequest,
  rejectRequest,
  resolveRequest,
  startRequest,
} from '@/hooks/request-state.mjs';

const useInstantVector = (configs, parseQuery = (query) => query, times) => {
  const data = ref(
    configs.map((config) => ({
      ...config,
      ...createRequestState(config.data),
    })),
  );
  const rangeRequestIds = configs.map(() => 0);

  const safeParseQuery = (q) => {
    try {
      const parsed = parseQuery(q);
      return typeof parsed === 'string' ? parsed : undefined;
    } catch {
      return undefined;
    }
  };

  const parseConfigQueries = () =>
    configs.map(({ query, totalQuery, percentQuery }) => ({
      query: query ? safeParseQuery(query) : undefined,
      totalQuery: totalQuery ? safeParseQuery(totalQuery) : undefined,
      percentQuery: percentQuery ? safeParseQuery(percentQuery) : undefined,
    }));

  const fetchInstantData = async (parsedConfigs) => {
    const reqs = configs.map(
      async (config, index) => {
        const { query, totalQuery } = config;
        const parsedConfig = parsedConfigs[index];
        const metric = data.value[index];
        const requiresCapacity = requiresMetricCapacity(config);
        const hasResolved = metric.hasResolved;
        const nextMetric = snapshotMetricState(metric);
        const requestId = startRequest(metric, { hasResolved });
        const parsedQuery = parsedConfig.query;
        const parsedQueryStatus = classifyParsedQuery(parsedQuery);
        const parsedTotalQueryStatus = totalQuery
          ? classifyParsedQuery(parsedConfig.totalQuery)
          : PARSED_QUERY_STATUS.READY;

        if (
          parsedQueryStatus === PARSED_QUERY_STATUS.PENDING ||
          parsedTotalQueryStatus === PARSED_QUERY_STATUS.PENDING
        ) {
          resetMetricQueryState(metric, config, {
            requestId,
            status: METRIC_STATUS.LOADING,
          });
          return;
        }
        if (
          parsedQueryStatus === PARSED_QUERY_STATUS.INVALID ||
          parsedTotalQueryStatus === PARSED_QUERY_STATUS.INVALID
        ) {
          resetMetricQueryState(metric, config, {
            requestId,
            status: METRIC_STATUS.INVALID,
          });
          return;
        }
        try {
          let usedStatus = METRIC_STATUS.MISSING;
          let totalStatus;
          if (query) {
            const usedData = await cardApi.getInstantVector({
              query: parsedQuery,
            });
            if (!isLatestRequest(metric, requestId)) return;
            const usedSample = readInstantValue(usedData);

            // Preserve whether Prometheus returned a real sample. A zero sample
            // is valid; an empty vector means that the metric is unavailable.
            nextMetric.hasData = usedSample.status === METRIC_STATUS.READY;
            nextMetric.count = usedSample.value;
            nextMetric.used = usedSample.value;
            usedStatus = usedSample.status;
          }

          if (totalQuery) {
            const parsedTotalQuery = parsedConfig.totalQuery;
            const totalData = await cardApi.getInstantVector({
              query: parsedTotalQuery,
            });
            if (!isLatestRequest(metric, requestId)) return;
            const totalSample = readInstantValue(totalData);
            totalStatus = totalSample.status;
            nextMetric.totalHasData = totalSample.status === METRIC_STATUS.READY;
            nextMetric.total = totalSample.value;
          }

          nextMetric.status = resolveMetricStatus({
            usedStatus,
            totalStatus,
            total: nextMetric.total,
            requiresCapacity,
          });

          if (requiresCapacity) {
            const percent = calculateMetricPercent(
              nextMetric.used,
              nextMetric.total,
            );
            nextMetric.percent = percent ?? 0;
          }

          if (!isLatestRequest(metric, requestId)) return;
          restoreMetricState(metric, nextMetric);
          resolveRequest(metric, {
            status: nextMetric.status,
            requestId,
          });
        } catch (error) {
          if (!isLatestRequest(metric, requestId)) return;
          if (!hasResolved) {
            setMetricErrorState(metric, {
              hasTotal: Boolean(totalQuery),
              requiresCapacity,
            });
          }
          rejectRequest(metric, error, { hasResolved, requestId });
        }
      },
    );

    await Promise.all(reqs);
  };

  const fetchRangeData = async (parsedConfigs = parseConfigQueries()) => {
    const reqs = configs.map(
      async ({ percentQuery }, index) => {
        const requestId = ++rangeRequestIds[index];
        if (percentQuery && times?.value?.[0] && times?.value?.[1]) {
          const parsedPercentQuery = parsedConfigs[index]?.percentQuery;
          if (
            classifyParsedQuery(parsedPercentQuery) !==
            PARSED_QUERY_STATUS.READY
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
            if (requestId !== rangeRequestIds[index]) return;

            data.value[index].data = percentData.data[0]?.values || [];
          } catch {
            // Keep the last successful series during a failed range refresh.
          }
        }
      },
    );

    await Promise.all(reqs);
  };

  const parsedConfigs = computed(parseConfigQueries);
  watch(
    parsedConfigs,
    (value) => {
      fetchInstantData(value);
      fetchRangeData(value);
    },
    { immediate: true, deep: true },
  );

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
