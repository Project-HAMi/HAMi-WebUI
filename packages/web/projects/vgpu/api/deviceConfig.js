import request from '@/utils/request';

const apiPrefix = '/api/vgpu';

class DeviceConfigApi {
  getDeviceConfig(signal) {
    return request({
      url: apiPrefix + '/v1/device-config',
      method: 'get',
      errorFeedback: 'inline',
      signal,
    });
  }
}

export default new DeviceConfigApi();
