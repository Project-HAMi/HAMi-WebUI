import { computed, onScopeDispose, ref, unref, watch } from 'vue';
import apiClient from '../api/client.mjs';
import { REQUEST_STATUS } from './request-state.mjs';
import { formatQueryTimestamp } from '../utils/query-time.mjs';

const VALID_SCOPES = new Set(['cluster', 'node', 'device', 'container']);

function toFinitePointValue(rawValue) {
  if (rawValue === null || rawValue === undefined || rawValue === '') return null;
  if (typeof rawValue !== 'number' && typeof rawValue !== 'string') return null;
  const value = Number(rawValue);
  return Number.isFinite(value) ? value : null;
}

function classifySeriesStatus(series, points) {
  if (!series || typeof series !== 'object') return REQUEST_STATUS.ERROR;
  const state = series.state?.state;
  const reason = series.state?.reason;
  if (state === 'known') {
    return points.some((point) => point.value !== null)
      ? REQUEST_STATUS.READY
      : REQUEST_STATUS.MISSING;
  }
  if (state === 'unknown' && reason === 'no_data') return REQUEST_STATUS.MISSING;
  if (state === 'conflict' || reason === 'invalid_response') return REQUEST_STATUS.INVALID;
  return REQUEST_STATUS.ERROR;
}

/**
 * Project a TrendsReply from /api/v1/telemetry/trends into chart series states.
 * @param {object} reply
 * @param {Array<{ key: string, metric: string }>} metricConfigs
 */
export function projectTrendSeries(reply, metricConfigs = []) {
  const byMetric = new Map();
  if (Array.isArray(reply?.series)) {
    for (const item of reply.series) {
      if (item && typeof item.metric === 'string' && !byMetric.has(item.metric)) {
        byMetric.set(item.metric, item);
      }
    }
  }

  return metricConfigs.map((config) => {
    const rawSeries = byMetric.get(config.metric);
    const data = Array.isArray(rawSeries?.points)
      ? rawSeries.points.map((point) => {
          const timestamp = Date.parse(point?.at);
          return {
            timestamp: Number.isFinite(timestamp) ? timestamp : null,
            value: toFinitePointValue(point?.value),
          };
        })
      : [];
    const status = classifySeriesStatus(rawSeries, data);
    return {
      ...config,
      status,
      data: status === REQUEST_STATUS.READY || status === REQUEST_STATUS.MISSING ? data : [],
    };
  });
}

function hasDrawableOutcome(seriesList) {
  return (
    seriesList.some((item) => item.status === REQUEST_STATUS.READY) ||
    (seriesList.length > 0 && seriesList.every((item) => item.status === REQUEST_STATUS.MISSING))
  );
}

const groupKey = (item) => item?.sectionIndex ?? '__default__';
const isFailedStatus = (status) => status === REQUEST_STATUS.ERROR || status === REQUEST_STATUS.INVALID;

function groupIndicesBySection(metricConfigs) {
  const groups = new Map();
  metricConfigs.forEach((config, index) => {
    const key = groupKey(config);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(index);
  });
  return groups;
}

function initialSeries(metricConfigs) {
  return metricConfigs.map((config) => ({
    ...config,
    status: REQUEST_STATUS.LOADING,
    data: [],
    refreshing: false,
    refreshError: null,
  }));
}

function hasRequiredScopeParams(scope, params) {
  if (!VALID_SCOPES.has(scope)) return false;
  if (scope === 'cluster') return true;
  if (scope === 'node') return Boolean(params?.node && params?.nodeUid);
  if (scope === 'device') return Boolean(params?.deviceId);
  if (scope === 'container') {
    return Boolean(params?.namespace && params?.pod && params?.container && params?.podUid);
  }
  return false;
}

/**
 * Subscribe a reactive chart series list to /api/v1/telemetry/trends.
 */
export function useTelemetryTrends({ scope, params, metrics = [], times, client = apiClient }) {
  const data = ref(initialSeries(metrics));
  let requestId = 0;
  let activeController = null;
  let resolvedScopeKey = null;
  let drawableGroups = new Set();
  const metricGroups = groupIndicesBySection(metrics);

  const resolvedParams = computed(() => {
    const raw = unref(params);
    return raw && typeof raw === 'object' ? raw : {};
  });

  const scopeKey = computed(() => JSON.stringify([unref(scope), resolvedParams.value]));

  const abortActive = () => {
    if (activeController) {
      activeController.abort();
      activeController = null;
    }
  };

  onScopeDispose(abortActive);

  const fetchTrends = async () => {
    const start = times?.value?.[0];
    const end = times?.value?.[1];
    if (!start || !end) return;

    const currentScope = unref(scope);
    const currentParams = resolvedParams.value;
    const currentKey = scopeKey.value;

    abortActive();
    const myRequestId = ++requestId;

    if (!hasRequiredScopeParams(currentScope, currentParams)) {
      resolvedScopeKey = null;
      drawableGroups = new Set();
      data.value = initialSeries(metrics);
      return;
    }

    const sameScope = resolvedScopeKey === currentKey;
    if (!sameScope) drawableGroups = new Set();
    data.value = metrics.map((config, index) => (
      sameScope && drawableGroups.has(groupKey(config))
        ? { ...data.value[index], refreshing: true, refreshError: null }
        : { ...config, status: REQUEST_STATUS.LOADING, data: [], refreshing: false, refreshError: null }
    ));

    const controller = new AbortController();
    activeController = controller;

    try {
      const reply = await client.getTelemetryTrends(
        {
          scope: currentScope,
          ...currentParams,
          start: formatQueryTimestamp(start),
          end: formatQueryTimestamp(end),
        },
        { signal: controller.signal },
      );
      if (controller.signal.aborted || myRequestId !== requestId) return;

      const projected = projectTrendSeries(reply, metrics);
      const nextSeries = new Array(metrics.length);
      for (const [gKey, indices] of metricGroups.entries()) {
        const wasDrawable = sameScope && drawableGroups.has(gKey);
        const groupProjected = indices.map((index) => projected[index]);
        if (wasDrawable && groupProjected.some((item) => isFailedStatus(item.status))) {
          for (const index of indices) {
            nextSeries[index] = {
              ...data.value[index],
              refreshing: false,
              refreshError: new Error('Telemetry refresh unavailable'),
            };
          }
          continue;
        }
        for (const index of indices) {
          nextSeries[index] = {
            ...projected[index],
            refreshing: false,
            refreshError: null,
          };
        }
        if (hasDrawableOutcome(groupProjected)) drawableGroups.add(gKey);
        else drawableGroups.delete(gKey);
      }

      data.value = nextSeries;
      resolvedScopeKey = currentKey;
    } catch (error) {
      if (controller.signal.aborted || myRequestId !== requestId) return;
      const nextSeries = new Array(metrics.length);
      for (const [gKey, indices] of metricGroups.entries()) {
        const wasDrawable = sameScope && drawableGroups.has(gKey);
        for (const index of indices) {
          nextSeries[index] = wasDrawable
            ? { ...data.value[index], refreshing: false, refreshError: error }
            : { ...metrics[index], status: REQUEST_STATUS.ERROR, data: [], refreshing: false, refreshError: null };
        }
        if (!wasDrawable) drawableGroups.delete(gKey);
      }
      data.value = nextSeries;
      resolvedScopeKey = currentKey;
    } finally {
      if (activeController === controller) activeController = null;
    }
  };

  watch([() => times?.value, scopeKey], () => {
    fetchTrends();
  }, { immediate: true });

  return {
    data,
    refresh: fetchTrends,
  };
}
