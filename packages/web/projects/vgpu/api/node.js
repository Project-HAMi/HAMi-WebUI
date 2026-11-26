import request from '@/utils/request';

const apiPrefix = '/api/vgpu';

// Older servers can accept the inventory option without recognizing it.
const requireInventoryCoverage = (reply, entity) => {
  if (reply?.inventoryCoverage?.entity !== entity) throw new Error('Node inventory coverage is missing');
  return reply;
};

class nodeApi {
  getNodeList(data) {
    return {
      url: apiPrefix +  '/v1/nodes',
      method: 'POST',
      data,
    };
  }

  async getInventoryNodes() {
    const reply = await request({ ...this.getNodeList({ useResourceInventory: true }), errorFeedback: 'inline' });
    return requireInventoryCoverage(reply, 'node');
  }

  getNodes(data) {
    return request({
      url: apiPrefix +  '/v1/nodes',
      method: 'POST',
      data,
    });
  }

  async getInventoryNode(uid, signal) {
    try {
      const reply = await request({
        url: apiPrefix + '/v1/node',
        method: 'GET',
        params: { uid, useResourceInventory: true },
        errorFeedback: 'inline',
        signal,
      });
      return requireInventoryCoverage(reply, 'device');
    } catch (error) {
      if (error.response?.status === 404 && error.response.data?.reason === 'RESOURCE_NODE_NOT_FOUND') {
        return {};
      }
      throw error;
    }
  }

  getNodeListReq(data) {
    return request(this.getNodeList(data));
  }

  async getNodeDevices({ name: nodeName, uid: nodeUid }, signal) {
    const reply = await request({
      url: apiPrefix + '/v1/gpus',
      method: 'POST',
      data: { filters: { nodeName, nodeUid } },
      errorFeedback: 'inline',
      signal,
    });
    // Older servers can ignore the UID filter and return name-only reservations.
    if (!reply?.list?.every(device => typeof device?.nodeAllocationComplete === 'boolean')) {
      throw new Error('Node-scoped device allocations are unavailable');
    }
    return reply;
  }

  async getNodeAllocatedContainers({ name: nodeName, uid: nodeUid }, signal) {
    const reply = await request({
      url: apiPrefix + '/v1/containers',
      method: 'POST',
      data: { filters: { nodeName, nodeUid }, useNodeAllocation: true },
      errorFeedback: 'inline',
      signal,
    });
    if (reply?.allocationNodeUid !== nodeUid) throw new Error('Node-scoped holders are unavailable');
    return reply;
  }
}

export default new nodeApi();
