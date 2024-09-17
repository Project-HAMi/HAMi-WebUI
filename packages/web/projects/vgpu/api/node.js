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
}

export default new nodeApi();
