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
  }));

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

  const refresh = async () => {
    const currentRequestId = ++requestId;
    series.value = createSeries(REQUEST_STATUS.LOADING);

    const currentSource = unref(source);
    const currentRange = unref(range);
    if (!isValidSource(currentSource) || !isValidRange(currentRange)) {
      if (currentRequestId === requestId) {
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
    series.value = createSeries(
      REQUEST_STATUS.MISSING,
      Object.fromEntries(outcomes),
    );
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
