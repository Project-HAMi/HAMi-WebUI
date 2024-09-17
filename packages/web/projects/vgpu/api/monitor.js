import request from '@/utils/request';

const apiPrefix = '/api/vgpu';


class monitorApi {
  summary(data) {
    return request({ apiPrefix, url: apiPrefix +  '/v1/summary', method: 'POST', data });
  }

  usage(data) {
    return request({
      url: apiPrefix +  '/v1/monitor/summary',
      method: 'POST',
      data,
    });
  }
}

export default new monitorApi();
