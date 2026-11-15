import request from '@/utils/request';

const read = (path, signal) => request({
  url: `/api/v1/${path}`, method: 'GET', signal, errorFeedback: 'inline',
});

export default {
  overviewInventory: signal => read('overview/inventory', signal),
};
