import request from '@/utils/request';

const apiPrefix = '/api/vgpu';


class nodeApi {
  getNodeList(data) {
    return {
      url: apiPrefix +  '/v1/nodes',
      method: 'POST',
      data,
    };
  }

  getNodes(data) {
    return request({
      url: apiPrefix +  '/v1/nodes',
      method: 'POST',
      data,
    });
  }

  getNodeDetail(params) {
    return request({
      url: apiPrefix +  '/v1/node',
      method: 'GET',
      params,
    });
  }

  getNodeListReq(data) {
    return request(this.getNodeList(data));
  }

  getNodeDevices(nodeName, signal) {
    return request({
      url: apiPrefix + '/v1/gpus',
      method: 'POST',
      data: { filters: { nodeName } },
      errorFeedback: 'inline',
      signal,
    });
  }

  getNodeAllocatedContainers(nodeUid, signal) {
    return request({
      url: apiPrefix + '/v1/containers',
      method: 'POST',
      data: { filters: { nodeUid } },
      errorFeedback: 'inline',
      signal,
    });
  }
}

export default new nodeApi();
