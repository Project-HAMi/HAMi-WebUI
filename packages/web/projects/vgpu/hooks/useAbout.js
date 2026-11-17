import { reactive } from 'vue';
import { createRequestState, rejectRequest, resolveRequest, startRequest } from '@/hooks/request-state.mjs';
import request from '@/utils/request';

export default function useAbout() {
  const state = reactive(createRequestState(null));

  const read = async () => {
    const hasResolved = state.data !== null;
    const requestId = startRequest(state, { hasResolved });
    try {
      const data = await request({ url: '/api/vgpu/v1/about', method: 'get', errorFeedback: 'inline' });
      // Some deployments return the SPA's index page for unregistered routes.
      if (typeof data === 'string' || !data?.checkedAt) {
        resolveRequest(state, { requestId, data: null, status: 'missing' });
        return;
      }
      resolveRequest(state, { data, requestId });
    } catch (error) {
      if (error.response?.status === 404) {
        resolveRequest(state, { requestId, data: null, status: 'missing' });
      } else {
        rejectRequest(state, error, { hasResolved, requestId });
      }
    }
  };

  let pending;
  function load() {
    pending ??= read().finally(() => { pending = undefined; });
    return pending;
  }

  return { state, load };
}
