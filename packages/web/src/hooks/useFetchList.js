import { onMounted, reactive, toRefs } from 'vue';
import { isArray } from 'lodash';
import {
  createRequestState,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from './request-state.mjs';

const useFetchList = (req, path = 'list') => {
  const state = reactive(createRequestState([]));

  const fetchList = async () => {
    const hasResolved = state.hasResolved;
    const requestId = startRequest(state, { hasResolved });
    try {
      const response = await (typeof req === 'function' ? req() : req);
      const listData = response?.[path];
      if (!isArray(listData)) {
        resolveRequest(state, {
          data: [],
          status: REQUEST_STATUS.INVALID,
          requestId,
        });
        return;
      }
      resolveRequest(state, {
        data: listData,
        status: REQUEST_STATUS.READY,
        requestId,
      });
    } catch (error) {
      rejectRequest(state, error, { hasResolved, requestId });
    }
  };

  onMounted(fetchList);

  return { ...toRefs(state), refresh: fetchList };
};

export default useFetchList;
