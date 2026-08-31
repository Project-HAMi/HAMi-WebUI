import { onMounted, reactive, toRefs } from 'vue';
import { isArray } from 'lodash';
import {
  createRequestState,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from './request-state.mjs';

const useFetchList = (req, pathOrOptions = 'list') => {
  const options = typeof pathOrOptions === 'string'
    ? { path: pathOrOptions }
    : pathOrOptions;
  const {
    immediate = true,
    mapData = (data) => data,
    path = 'list',
  } = options;
  const state = reactive(createRequestState([]));

  const fetchList = async () => {
    // Only a ready collection is usable stale content. Retrying a blocking
    // error or invalid response should return to the initial loading state.
    const hasResolved = state.hasResolved && state.status === REQUEST_STATUS.READY;
    const requestId = startRequest(state, { hasResolved });
    try {
      const response = await (typeof req === 'function' ? req() : req);
      const listData = response?.[path];
      if (!isArray(listData)) {
        rejectRequest(state, new TypeError(`Expected response.${path} to be an array`), {
          hasResolved,
          requestId,
          status: REQUEST_STATUS.INVALID,
        });
        return;
      }
      const mappedData = mapData(listData);
      if (!isArray(mappedData)) {
        rejectRequest(state, new TypeError('Expected mapData to return an array'), {
          hasResolved,
          requestId,
          status: REQUEST_STATUS.INVALID,
        });
        return;
      }
      resolveRequest(state, {
        data: mappedData,
        status: REQUEST_STATUS.READY,
        requestId,
      });
    } catch (error) {
      rejectRequest(state, error, { hasResolved, requestId });
    }
  };

  if (immediate) {
    onMounted(fetchList);
  }

  return { ...toRefs(state), refresh: fetchList };
};

export default useFetchList;
