import request from '@/utils/request';
import resourcesApi from './resources';
import { physicalDeviceKey, readOverviewInventory } from './overview-inventory.mjs';
import { physicalModelDistribution } from './model-distribution.mjs';
import { matchDeviceRegistration, registrationDetail } from './device-detail-link.mjs';
import { normalizeRangeVectorResponse } from '../metrics/range-vector-state.mjs';

const apiPrefix = '/api/vgpu';

class cardApi {
  getCardList(data = { filters: {} }) {
    return {
      url: apiPrefix + '/v1/gpus',
      method: 'POST',
      data,
    };
  }

  async getPhysicalCardList() {
    const [inventory, registrations] = await Promise.all([
      resourcesApi.overviewInventory().then(readOverviewInventory),
      request({ ...this.getCardList(), errorFeedback: 'inline' }).catch(() => null),
    ]);
    if (!inventory.deviceCoverage.complete && !inventory.devices.length) throw new Error('Physical inventory unavailable');
    const byNode = new Map(inventory.nodes.map(node => [node.name, node.uid]));
    const match = matchDeviceRegistration(Array.isArray(registrations?.list) ? registrations.list : []);
    const list = inventory.devices.map(device => {
      const row = match({ ...device, nodeUid: byNode.get(device.node) }, device.deviceId);
      return {
        ...row, physicalKey: physicalDeviceKey(device), uuid: device.deviceId,
        nodeName: device.node, vendor: device.provider, type: device.model, health: device.healthy,
        utilizationRatio: device.utilizationRatio, memoryUsageRatio: device.memoryUsageRatio,
        detailTo: registrationDetail(row),
      };
    });
    return { list, models: physicalModelDistribution(inventory.devices, inventory.deviceCoverage) };
  }

  getCardType(data = { filters: {} }) {
    return {
      url: apiPrefix + '/v1/gpu-types',
      method: 'POST',
      data,
    };
  }

  getCardListReq(data) {
    return request(this.getCardList(data));
  }

  getCardTypeReq(data = { filters: {} }) {
    return request({ ...this.getCardType(data), errorFeedback: 'inline' });
  }

  // The registration detail returns a boolean even when its health field was absent.
  async getDetailHealth(card, signal) {
    const inventory = readOverviewInventory(await resourcesApi.overviewInventory(signal));
    const node = inventory.nodes.find(row => row.name === card.nodeName);
    if (!node?.uid || node.uid !== card.nodeUid) return {};
    const match = matchDeviceRegistration([card]);
    const device = inventory.devices.find(row =>
      row.node === node.name && match({ ...row, nodeUid: node.uid }, row.deviceId));
    if (device) return { health: device.healthy };
    // Ascend910C registers modules, so its node detail can show the module's own
    // registration. An unmatched physical ID provides no health evidence.
    const moduleRegistration = card.vendor === 'Ascend' && card.type === 'Ascend910C';
    return moduleRegistration ? card : {};
  }

  async getCardDetail(params, signal) {
    try {
      return await request({
        url: apiPrefix + '/v1/gpu',
        method: 'get',
        errorFeedback: 'inline',
        params,
        signal,
      });
    } catch (error) {
      if (error.response?.status === 404 && error.response.data?.reason === 'RESOURCE_GPU_NOT_FOUND') {
        return {};
      }
      throw error;
    }
  }

  getRangeVector(data) {
    return request({
      url: apiPrefix + '/v1/monitor/query/range-vector',
      method: 'post',
      data,
    }).then(normalizeRangeVectorResponse);
  }

  getInstantVector(data, signal) {
    return request({
      url: apiPrefix + '/v1/monitor/query/instant-vector',
      signal,
      errorFeedback: signal ? 'inline' : undefined,
      method: 'post',
      data,
    });
  }
}

export default new cardApi();
