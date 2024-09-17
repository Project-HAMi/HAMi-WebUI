import request from '@/utils/request';

const apiPrefix = '/api/vgpu';


class taskApi {
  getTaskList(data) {
    return {
      url: apiPrefix +  '/v1/containers',
      method: 'POST',
      dataPath: 'items',
      data,
    };
  }

  getTaskListReq(data) {
    return request({
      url: apiPrefix +  '/v1/containers',
      method: 'POST',
      dataPath: 'items',
      data,
    });
  }

  getTaskDetail(params) {
    return request({
      url: apiPrefix +  '/v1/container',
      method: 'get',
      params,
    });
  }

  deleteTask(data) {
    return request({
      url: apiPrefix +  '/v1/task/delete',
      method: 'post',
      data,
    });
  }
}

export default new taskApi();
