import { onScopeDispose, ref, unref, watch } from 'vue';
import apiClient from '@/api/client.mjs';
import { createNodeDeviceAllocationLoader } from './node-device-allocation.mjs';

export default function useNodeDeviceAllocation(source) {
  const entries = ref([]);
  const status = ref('idle');
  const loader = createNodeDeviceAllocationLoader({
    getDevices: async (node, signal) => {
      const reply = await apiClient.listDevices({ nodeName: node.name, nodeUid: node.uid }, { signal });
      // Reject un-scoped legacy payloads that omit nodeAllocationComplete.
      if (!reply?.list?.every((device) => typeof device?.nodeAllocationComplete === 'boolean')) {
        throw new Error('Node-scoped device allocations are unavailable');
      }
      return reply;
    },
    getContainers: async (node, signal) => {
      const reply = await apiClient.listNodeContainers(node.uid, node.name, { signal });
      if (reply?.allocationNodeUid !== node.uid) {
        throw new Error('Node-scoped holders are unavailable');
      }
      return reply;
    },
    onChange: (state) => {
      entries.value = state.entries;
      status.value = state.status;
    },
  });
  watch(source, (node) => loader.load(node), { immediate: true });
  onScopeDispose(loader.dispose);
  return { entries, status, retry: () => loader.load(unref(source)) };
}
