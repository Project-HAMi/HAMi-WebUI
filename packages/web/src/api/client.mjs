import { getBasePath } from '../utils/base-path.mjs';

export class ApiClientError extends Error {
  constructor({ status, code, reason = '', message = '', metadata = {}, url = '' }) {
    super(message || reason || `HTTP ${status}`);
    this.name = 'ApiClientError';
    this.status = status;
    this.code = code;
    this.reason = reason;
    this.metadata = metadata;
    this.url = url;
  }
}

function requireNonEmpty(name, value) {
  const trimmed = typeof value === 'string' ? value.trim() : '';
  if (!trimmed) {
    throw new TypeError(`${name} is required`);
  }
  return trimmed;
}

function requireSegment(name, value) {
  return encodeURIComponent(requireNonEmpty(name, value));
}

export function buildApiUrl(path, query, basePath = getBasePath()) {
  const cleanPath = String(path || '').replace(/^\/+/, '');
  const prefix = basePath.endsWith('/') ? basePath : `${basePath}/`;
  const search = new URLSearchParams();
  if (query && typeof query === 'object') {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== '') {
        search.set(key, String(value));
      }
    }
  }
  const qs = search.toString();
  const baseUrl = `${prefix}api/v1/${cleanPath}`;
  return qs ? `${baseUrl}?${qs}` : baseUrl;
}

export function createApiClient({
  fetchImpl = globalThis.fetch,
  getBasePathImpl = getBasePath,
} = {}) {
  if (typeof fetchImpl !== 'function') {
    throw new TypeError('fetch implementation is required');
  }

  async function requestJson(path, { query, signal } = {}) {
    const url = buildApiUrl(path, query, getBasePathImpl());
    const response = await fetchImpl(url, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      signal,
    });
    const raw = await response.text();
    let payload = null;
    if (raw) {
      try {
        payload = JSON.parse(raw);
      } catch {
        throw new ApiClientError({
          status: response.status,
          reason: 'INVALID_JSON',
          message: `Invalid JSON response from ${url}`,
          url,
        });
      }
    }
    if (!response.ok || payload == null || typeof payload !== 'object') {
      const errorBody = payload && typeof payload === 'object' ? payload : {};
      throw new ApiClientError({
        status: response.status,
        code: errorBody.code,
        reason: errorBody.reason || (!response.ok ? 'HTTP_ERROR' : 'EMPTY_RESPONSE'),
        message: errorBody.message || response.statusText || `HTTP ${response.status}`,
        metadata: errorBody.metadata && typeof errorBody.metadata === 'object' ? errorBody.metadata : {},
        url,
      });
    }
    return payload;
  }

  return {
    async getOverviewInventory(options = {}) {
      return requestJson('overview/inventory', { signal: options.signal });
    },

    async getTelemetryTrends(query, options = {}) {
      if (!query || typeof query !== 'object') {
        throw new TypeError('trends query is required');
      }
      return requestJson('telemetry/trends', {
        query: {
          scope: requireNonEmpty('scope', query.scope),
          node: query.node,
          node_uid: query.nodeUid,
          device_id: query.deviceId,
          namespace: query.namespace,
          pod: query.pod,
          container: query.container,
          pod_uid: query.podUid,
          start: query.start,
          end: query.end,
          step: query.step,
        },
        signal: options.signal,
      });
    },

    async listNodes(query = {}, options = {}) {
      return requestJson('nodes', {
        query: {
          ip: query.ip,
          type: query.type,
          is_schedulable: query.isSchedulable,
        },
        signal: options.signal,
      });
    },

    async getNode(uid, options = {}) {
      return requestJson(`nodes/${requireSegment('uid', uid)}`, { signal: options.signal });
    },

    async listNodeContainers(uid, nodeName, options = {}) {
      return requestJson(`nodes/${requireSegment('uid', uid)}/containers`, {
        query: { node_name: requireNonEmpty('nodeName', nodeName) },
        signal: options.signal,
      });
    },

    async listDevices(query = {}, options = {}) {
      return requestJson('devices', {
        query: {
          id: query.id,
          type: query.type,
          node_name: query.nodeName,
          node_uid: query.nodeUid,
          provider: query.provider,
        },
        signal: options.signal,
      });
    },

    async getDevice(id, options = {}) {
      return requestJson('device', {
        query: {
          id: requireNonEmpty('id', id),
          node_uid: options.nodeUid,
        },
        signal: options.signal,
      });
    },

    async listContainers(query = {}, options = {}) {
      return requestJson('containers', {
        query: {
          name: query.name,
          node_name: query.nodeName,
          status: query.status,
          device_id: query.deviceId,
          node_uid: query.nodeUid,
          namespace: query.namespace,
          priority: query.priority,
          keyword: query.keyword,
          page: query.page,
          page_size: query.pageSize,
        },
        signal: options.signal,
      });
    },

    async getContainerRankings(options = {}) {
      return requestJson('containers/rankings', { signal: options.signal });
    },

    async getContainer(podUid, container, options = {}) {
      return requestJson(
        `containers/${requireSegment('podUid', podUid)}/${requireSegment('container', container)}`,
        {
          query: { device_id: options.deviceId },
          signal: options.signal,
        },
      );
    },

    async getSchedulingPod(namespace, name, uid, options = {}) {
      return requestJson(
        `scheduling/pods/${requireSegment('namespace', namespace)}/${requireSegment('name', name)}`,
        {
          query: { uid: requireNonEmpty('uid', uid) },
          signal: options.signal,
        },
      );
    },

    async getDeviceConfig(options = {}) {
      return requestJson('device-config', { signal: options.signal });
    },

    async getAbout(options = {}) {
      return requestJson('about', { signal: options.signal });
    },
  };
}

const apiClient = typeof globalThis.fetch === 'function' ? createApiClient() : null;
export default apiClient;
