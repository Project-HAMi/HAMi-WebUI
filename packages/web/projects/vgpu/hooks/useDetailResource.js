import { getCurrentScope, onScopeDispose, reactive, toRefs, unref, watch } from 'vue';

import { createRequestState, REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  rejectDetailResourceRequest,
  resolveDetailResourceRequest,
  startDetailResourceRequest,
} from './detail-resource-state.mjs';

const defaultEntityKey = (value) => {
  if (value === undefined || value === null) return '';
  if (typeof value === 'object') {
    const sorted = Object.keys(value)
      .sort()
      .reduce((acc, key) => {
        acc[key] = value[key];
        return acc;
      }, {});
    return JSON.stringify(sorted);
  }
  return String(value);
};

const useDetailResource = ({
  source,
  request,
  classify,
  entityKey: getEntityKey = defaultEntityKey,
  isValidSource = (value) => value !== undefined && value !== null && value !== '',
  initialData = () => ({}),
}) => {
  const createInitialData =
    typeof initialData === 'function' ? initialData : () => initialData;
  const state = reactive(
    createRequestState(createInitialData(), {
      entityKey: getEntityKey(unref(source)),
    }),
  );

  let activeController = null;
  const abortActive = () => {
    if (activeController) {
      activeController.abort();
      activeController = null;
    }
  };
  if (getCurrentScope()) {
    onScopeDispose(abortActive);
  }

  const load = async (sourceValue = unref(source)) => {
    abortActive();
    const emptyData = createInitialData();
    const currentEntityKey = getEntityKey(sourceValue);
    const requestId = startDetailResourceRequest(state, emptyData, {
      entityKey: currentEntityKey,
    });

    if (!isValidSource(sourceValue)) {
      resolveDetailResourceRequest(state, {
        data: emptyData,
        initialData: emptyData,
        status: REQUEST_STATUS.INVALID,
        requestId,
        entityKey: currentEntityKey,
      });
      return;
    }

    const controller = new AbortController();
    activeController = controller;
    try {
      const data = await request(sourceValue, controller.signal);
      resolveDetailResourceRequest(state, {
        data,
        initialData: emptyData,
        status: classify(data, sourceValue),
        requestId,
        entityKey: currentEntityKey,
      });
    } catch (error) {
      if (controller.signal.aborted || error?.code === 'ERR_CANCELED') {
        return;
      }
      rejectDetailResourceRequest(state, error, {
        requestId,
        entityKey: currentEntityKey,
      });
    } finally {
      if (activeController === controller) {
        activeController = null;
      }
    }
  };

  watch(source, load, { immediate: true });

  return {
    ...toRefs(state),
    retry: () => load(unref(source)),
  };
};

export default useDetailResource;
