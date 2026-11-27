import { reactive } from 'vue';
import apiClient from '@/api/client.mjs';
import { createRequestState, rejectRequest, resolveRequest, startRequest } from '@/hooks/request-state.mjs';

export default function useAbout() {
  const state = reactive(createRequestState(null));

  const read = async () => {
    const hasResolved = state.data !== null;
    const requestId = startRequest(state, { hasResolved });
    try {
      const data = await apiClient.getAbout();
      if (!data?.checkedAt) {
        resolveRequest(state, { requestId, data: null, status: 'missing' });
        return;
      }
      resolveRequest(state, { data, requestId });
    } catch (error) {
      // Some deployments return the SPA's index page (INVALID_JSON) or 404 for unregistered routes.
      if (error?.status === 404 || error?.reason === 'INVALID_JSON') {
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
