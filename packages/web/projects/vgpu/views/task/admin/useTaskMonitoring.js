import { computed, ref, unref, watch } from 'vue';

import { REQUEST_STATUS } from '../../../../../src/hooks/request-state.mjs';
import { readRangeVector } from '../../../hooks/range-vector-query-state.mjs';
import { buildTaskMonitoringQueries } from '../../../metrics/query-contract.mjs';
import { renderPromQLTemplate } from '../../../metrics/promql-template.mjs';

const SERIES = Object.freeze([
  { key: 'computeUsage', titleKey: 'task.computeUsageTrend' },
  { key: 'memoryUsage', titleKey: 'task.memUsageTrend' },
]);

const createSeries = (status, outcomes = {}) =>
  SERIES.map((definition) => ({
    ...definition,
    data: outcomes[definition.key]?.data || [],
    error: outcomes[definition.key]?.error || null,
    status: outcomes[definition.key]?.status || status,
    refreshing: false,
    refreshError: null,
  }));

const FAILED_STATUSES = new Set([REQUEST_STATUS.ERROR, REQUEST_STATUS.INVALID]);
// Only a real result, data or a confirmed empty one, is worth keeping through a failed refresh.
const KEPT_STATUSES = new Set([REQUEST_STATUS.READY, REQUEST_STATUS.MISSING]);

// The workload a result belongs to; another workload starts over instead of showing it.
const sourceScope = (source) => (isValidSource(source)
  ? [source.namespace, source.pod, source.podUid, source.container, source.expectedDeviceCount, source.expectedVgpuCount].join('\n')
  : null);

const hasText = (value) =>
  typeof value === 'string' && value.trim() !== '';

const isValidSource = (source) =>
  source &&
  hasText(source.container) &&
  hasText(source.namespace) &&
  hasText(source.pod) &&
  hasText(source.podUid) &&
  Number.isSafeInteger(source.expectedDeviceCount) &&
  source.expectedDeviceCount > 0 &&
  Number.isSafeInteger(source.expectedVgpuCount) &&
  source.expectedVgpuCount >= source.expectedDeviceCount;

const isValidRange = (range) =>
  range && hasText(range.start) && hasText(range.end) && hasText(range.step);

export const getTaskMonitoringAllocationShape = ({
  allocatedDevices,
  deviceIds,
} = {}) => {
  const expectedVgpuCount = Number(allocatedDevices);
  const normalizedDeviceIds = Array.isArray(deviceIds)
    ? deviceIds.map((id) => (typeof id === 'string' ? id.trim() : ''))
    : [];
  if (
    !Number.isSafeInteger(expectedVgpuCount) ||
    expectedVgpuCount <= 0 ||
    normalizedDeviceIds.length !== expectedVgpuCount ||
    normalizedDeviceIds.some((id) => !id)
  ) {
    return null;
  }

  return {
    expectedDeviceCount: new Set(normalizedDeviceIds).size,
    expectedVgpuCount,
  };
};

const useTaskMonitoring = ({ source, range, request }) => {
  const series = ref(createSeries(REQUEST_STATUS.LOADING));
  let requestId = 0;
  let resolvedScope = null;

  const refresh = async () => {
    const currentRequestId = ++requestId;
    const currentSource = unref(source);
    const currentRange = unref(range);
    // The same workload keeps its last result while a new range loads.
    const scope = sourceScope(currentSource);
    const keepPrevious = scope !== null && scope === resolvedScope;
    if (!keepPrevious) resolvedScope = null;
    series.value = keepPrevious
      ? series.value.map((item) => ({ ...item, refreshing: true, refreshError: null }))
      : createSeries(REQUEST_STATUS.LOADING);

    if (!isValidSource(currentSource) || !isValidRange(currentRange)) {
      if (currentRequestId === requestId) {
        resolvedScope = null;
        series.value = createSeries(REQUEST_STATUS.MISSING);
      }
      return;
    }

    let queries;
    try {
      queries = buildTaskMonitoringQueries({
        expectedDeviceCount: currentSource.expectedDeviceCount,
        expectedVgpuCount: currentSource.expectedVgpuCount,
      });
    } catch (error) {
      if (currentRequestId === requestId) {
        const invalid = Object.fromEntries(
          SERIES.map(({ key }) => [key, {
            data: [],
            error,
            status: REQUEST_STATUS.INVALID,
          }]),
        );
        resolvedScope = null;
        series.value = createSeries(REQUEST_STATUS.INVALID, invalid);
      }
      return;
    }

    const variables = {
      container: currentSource.container,
      container_pod_uuid: `${currentSource.container}:${currentSource.podUid}`,
      namespace: currentSource.namespace,
      pod: currentSource.pod,
    };
    const outcomes = await Promise.all(
      SERIES.map(async ({ key }) => {
        try {
          const response = await request({
            query: renderPromQLTemplate(queries[key], variables),
            range: { ...currentRange },
          });
          const state = readRangeVector(response);
          return [key, {
            ...state,
            data: state.status === REQUEST_STATUS.READY ? state.data : [],
            error: null,
          }];
        } catch (error) {
          return [key, {
            data: [],
            error,
            status: REQUEST_STATUS.ERROR,
          }];
        }
      }),
    );

    if (currentRequestId !== requestId) return;
    const settled = createSeries(REQUEST_STATUS.MISSING, Object.fromEntries(outcomes));
    // A chart whose refresh failed or returned unusable data keeps its last real result and says
    // so; a chart that had none shows the new failure, and one chart's failure leaves the other's result.
    series.value = settled.map((item, index) => {
      const previous = series.value[index];
      if (!keepPrevious || !FAILED_STATUSES.has(item.status) || !KEPT_STATUSES.has(previous.status)) return item;
      return { ...previous, refreshing: false, refreshError: item.error || new Error('Invalid monitoring data') };
    });
    resolvedScope = scope;
  };

  watch(
    () => [unref(source), unref(range)],
    () => {
      void refresh();
    },
    { deep: true, immediate: true },
  );

  return {
    data: computed(() => series.value),
    refresh,
  };
};

export default useTaskMonitoring;
