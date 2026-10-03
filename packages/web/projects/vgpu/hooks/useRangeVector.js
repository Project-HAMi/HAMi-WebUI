import cardApi from '~/vgpu/api/card';
import { computed, ref, watch } from 'vue';
import { timeParse, calculatePrometheusStep } from '@/utils';
import {
  REQUEST_STATUS,
} from '@/hooks/request-state.mjs';
import {
  createRangeGroupState,
  createRangeVectorOutcome,
  publishInitialRangeOutcome,
  readRangeVector,
  settleRangeGroupGeneration,
  startRangeGroupGeneration,
} from './range-vector-query-state.mjs';

const useRangeVector = (
  configs,
  parseQuery = (query) => query,
  times,
) => {
  const groupedConfigs = [];
  const groupIndexes = new Map();
  configs.forEach((config) => {
    const key = config.sectionIndex ?? 'default';
    let groupIndex = groupIndexes.get(key);
    if (groupIndex === undefined) {
      groupIndex = groupedConfigs.length;
      groupIndexes.set(key, groupIndex);
      groupedConfigs.push({ key, dataSource: [] });
    }
    groupedConfigs[groupIndex].dataSource.push(config);
  });

  const groups = ref(
    groupedConfigs.map(({ dataSource, key }) =>
      createRangeGroupState(dataSource, key),
    ),
  );
  const data = computed(() =>
    groups.value.flatMap((group) =>
      group.dataSource.map((series) => ({
        ...series,
        refreshing: group.refreshing,
        refreshError: group.refreshError,
      })),
    ),
  );

  const safeParseQuery = (query) => {
    try {
      const parsed = parseQuery(query);
      return typeof parsed === 'string' ? parsed : undefined;
    } catch {
      return undefined;
    }
  };

  const replaceGroup = (groupIndex, update) => {
    const current = groups.value[groupIndex];
    const next = update(current);
    if (next === current) return;
    const nextGroups = groups.value.slice();
    nextGroups[groupIndex] = next;
    groups.value = nextGroups;
  };

  const fetchGroup = async (groupIndex, range) => {
    const scope = groupedConfigs[groupIndex].dataSource
      .map(({ query }) => safeParseQuery(query) ?? '')
      .join('\n');
    const started = startRangeGroupGeneration(groups.value[groupIndex], range, scope);
    replaceGroup(groupIndex, () => started);
    const requestId = started.requestId;
    const isInitialLoad = !started.hasResolved;

    const outcomes = await Promise.all(
      groupedConfigs[groupIndex].dataSource.map(async ({ query }, seriesIndex) => {
        const parsedQuery = safeParseQuery(query);
        let outcome;

        if (!parsedQuery) {
          outcome = createRangeVectorOutcome(
            { data: [], status: REQUEST_STATUS.INVALID },
            new Error('Invalid range vector query'),
          );
        } else if (parsedQuery.includes('undefined')) {
          // The resource is not known yet, as while its detail loads: nothing to show.
          outcome = createRangeVectorOutcome({ data: [], status: REQUEST_STATUS.LOADING });
        } else {
          try {
            const response = await cardApi.getRangeVector({
              query: parsedQuery,
              range: { ...range },
            });
            const state = readRangeVector(response);
            outcome = createRangeVectorOutcome(
              state,
              state.status === REQUEST_STATUS.INVALID
                ? new Error('Invalid range vector response')
                : null,
            );
          } catch (error) {
            outcome = createRangeVectorOutcome(
              { data: [], status: REQUEST_STATUS.ERROR },
              error,
            );
          }
        }

        if (isInitialLoad) {
          replaceGroup(groupIndex, (group) =>
            publishInitialRangeOutcome(
              group,
              requestId,
              seriesIndex,
              outcome,
            ),
          );
        }
        return outcome;
      }),
    );

    replaceGroup(groupIndex, (group) =>
      settleRangeGroupGeneration(group, requestId, outcomes),
    );
  };

  const fetchData = async () => {
    const start = times?.value?.[0];
    const end = times?.value?.[1];
    if (!start || !end) return;

    const range = {
      start: timeParse(start),
      end: timeParse(end),
      step: calculatePrometheusStep(start, end),
    };
    await Promise.all(
      groupedConfigs.map((_, groupIndex) => fetchGroup(groupIndex, range)),
    );
  };

  watch(
    [() => times?.value, () => configs.map(({ query }) => safeParseQuery(query)).join('\n')],
    () => fetchData(),
    { immediate: true },
  );

  return { data, refresh: fetchData };
};

export default useRangeVector;
