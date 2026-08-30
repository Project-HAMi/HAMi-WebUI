import { reactive, toRefs, unref, watch } from 'vue';

import { createRequestState, REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  rejectDetailResourceRequest,
  resolveDetailResourceRequest,
  startDetailResourceRequest,
} from './detail-resource-state.mjs';

const useDetailResource = ({
  source,
  request,
  classify,
  isValidSource = (value) => value !== undefined && value !== null && value !== '',
  initialData = () => ({}),
}) => {
  const createInitialData =
    typeof initialData === 'function' ? initialData : () => initialData;
  const state = reactive(createRequestState(createInitialData()));

  const load = async (sourceValue = unref(source)) => {
    const emptyData = createInitialData();
    const requestId = startDetailResourceRequest(state, emptyData);

    if (!isValidSource(sourceValue)) {
      resolveDetailResourceRequest(state, {
        data: emptyData,
        initialData: emptyData,
        status: REQUEST_STATUS.INVALID,
        requestId,
      });
      return;
    }

    try {
      const data = await request(sourceValue);
      resolveDetailResourceRequest(state, {
        data,
        initialData: emptyData,
        status: classify(data, sourceValue),
        requestId,
      });
    } catch (error) {
      rejectDetailResourceRequest(state, error, requestId);
    }
  };

  watch(source, load, { immediate: true });

  return {
    ...toRefs(state),
    retry: () => load(unref(source)),
  };
};

export default useDetailResource;
