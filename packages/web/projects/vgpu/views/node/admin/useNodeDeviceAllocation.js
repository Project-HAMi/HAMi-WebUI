import { onScopeDispose, ref, unref, watch } from 'vue';
import nodeApi from '~/vgpu/api/node';
import { createNodeDeviceAllocationLoader } from './node-device-allocation.mjs';

export default function useNodeDeviceAllocation(source) {
  const entries = ref([]);
  const status = ref('idle');
  const loader = createNodeDeviceAllocationLoader({
    getDevices: (name, signal) => nodeApi.getNodeDevices(name, signal),
    getContainers: (uid, signal) => nodeApi.getNodeAllocatedContainers(uid, signal),
    onChange: (state) => {
      entries.value = state.entries;
      status.value = state.status;
    },
  });
  watch(source, (node) => loader.load(node), { immediate: true });
  onScopeDispose(loader.dispose);
  return { entries, status, retry: () => loader.load(unref(source)) };
}
