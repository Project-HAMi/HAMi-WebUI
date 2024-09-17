import request from '@/utils/request';

const apiPrefix = '/api/vgpu';

class InfoApi {
    getSysInfo(data) {
        return request({
            url: apiPrefix + '/v1/sys-info',
            method: 'POST',
            data
        });
    }
}

export default new InfoApi();