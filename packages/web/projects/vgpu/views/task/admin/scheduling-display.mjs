const REASONS = Object.freeze({
  CardInsufficientMemory: 'memory',
  CardInsufficientCore: 'core',
  CardComputeUnitsExhausted: 'coreExhausted',
  CardTimeSlicingExhausted: 'slots',
  NodeInsufficientDevice: 'devices',
  AllocatedCardsInsufficientRequest: 'devices',
  CardTypeMismatch: 'model',
  CardUuidMismatch: 'uuid',
  CardNotHealth: 'health',
  CardCordoned: 'cordoned',
  ExclusiveDeviceAllocateConflict: 'exclusive',
  NumaNotFit: 'numa',
  CardMigTopologyInfeasible: 'mig',
  ResourceQuotaNotFit: 'quota',
  CardNotFoundCustomFilterRule: 'customFilter',
  ModeNotFit: 'mode',
  InsufficientCPU: 'cpu',
  InsufficientHostMemory: 'hostMemory',
  ExtendedResourceUnavailable: 'extendedResource',
  UntoleratedTaint: 'taint',
  NodeAffinity: 'affinity',
  UnboundPVC: 'pvc',
  PVCNotFound: 'pvcMissing',
  VolumeNodeAffinity: 'volumeAffinity',
  NodeUnschedulable: 'nodeUnschedulable',
  PodAffinity: 'podAffinity',
  TooManyPods: 'podCapacity',
  HostPortConflict: 'hostPort',
  SchedulingGated: 'gated',
  NoFeedback: 'noFeedback',
  UnknownSchedulingReason: 'unknown',
});

export const getSchedulingReasons = (pod = {}, t) => [...new Set(
  (pod.reasonCodes || []).map((code) => REASONS[code] || 'unknown'),
)].map((key) => ({ key, label: t(`scheduling.reason.${key}`) }));

const hasContainerAllocation = (pod, containerName) => containerName
  ? (pod.allocatedContainers || []).includes(containerName) : (pod.allocatedContainers || []).length > 0;

export const getSchedulingSummary = (pod = {}, t, containerName) => {
  if (['terminating', 'finished'].includes(pod.stage)) return t(`scheduling.stageSummary.${pod.stage}`);
  if (pod.nodeName && !hasContainerAllocation(pod, containerName)) return t('scheduling.noAllocation');
  if (pod.nodeName || pod.stage === 'bound') return t('scheduling.boundSummary');
  if (['gated', 'unknown'].includes(pod.stage)) {
    return t(`scheduling.stageSummary.${pod.stage}`);
  }
  const reasons = getSchedulingReasons(pod, t);
  if (reasons.length > 1) return t('scheduling.multipleReasons');
  return reasons[0]?.label || t('scheduling.reason.noFeedback');
};

export const getSchedulingChecks = (pod = {}, t, containerName) => {
  if (['terminating', 'finished', 'unknown'].includes(pod.stage)) return [];
  if (pod.nodeName && !hasContainerAllocation(pod, containerName)) return [t('scheduling.check.allocation')];
  if (pod.nodeName || pod.stage === 'bound') return [];
  if (pod.stage === 'gated') return [t('scheduling.check.gates')];
  const checks = [...new Set((pod.reasonCodes || []).map((code) => {
    const reason = REASONS[code];
    if (reason === 'memory') return 'memory';
    if (['core', 'coreExhausted'].includes(reason)) return 'compute';
    if (['slots', 'devices'].includes(reason)) return 'resources';
    if (['model', 'uuid', 'exclusive', 'numa', 'mig', 'customFilter', 'mode'].includes(reason)) return 'deviceConstraints';
    if (['health', 'cordoned'].includes(reason)) return 'deviceHealth';
    if (reason === 'quota') return 'quota';
    if (reason === 'extendedResource') return 'extendedResource';
    if (reason === 'nodeUnschedulable') return 'nodeUnschedulable';
    if (['cpu', 'hostMemory', 'taint', 'affinity', 'podAffinity', 'podCapacity', 'hostPort'].includes(reason)) return 'node';
    if (['pvc', 'pvcMissing', 'volumeAffinity'].includes(reason)) return 'storage';
    if (reason === 'gated') return 'gates';
    return null;
  }).filter(Boolean))];
  const lacksExplanation = (pod.reasonCodes || []).every((code) => (
    !REASONS[code] || ['noFeedback', 'unknown'].includes(REASONS[code])
  ));
  if (pod.stage === 'waiting' && lacksExplanation && !['hami', 'mixed'].includes(pod.reasonSource)) {
    // Unrecognized feedback still proves the scheduler ran.
    const hasFeedback = (pod.reasonCodes || []).some((code) => code !== 'NoFeedback');
    checks.push(hasFeedback ? 'rawFeedback' : 'scheduler');
  }
  return checks.slice(0, 2).map((key) => t(`scheduling.check.${key}`));
};

export const formatSchedulingDuration = (from, now, t) => {
  const start = Date.parse(from);
  if (!Number.isFinite(start) || !Number.isFinite(now) || now < start) return '';
  const minutes = Math.floor((now - start) / 60_000);
  if (minutes < 1) return t('scheduling.duration.lessThanMinute');
  if (minutes < 60) return t('scheduling.duration.minutes', { count: minutes });
  if (minutes < 1440) return t('scheduling.duration.hours', { count: Math.floor(minutes / 60) });
  return t('scheduling.duration.days', { count: Math.floor(minutes / 1440) });
};

export const formatSchedulingWait = (createdAt, now, t) => {
  const duration = formatSchedulingDuration(createdAt, now, t);
  return duration ? t('scheduling.waitedFor', { duration }) : '';
};

// Tolerate a minute of clock skew; later timestamps return ''.
export const formatSchedulingAgo = (at, now, t) => {
  const time = Date.parse(at);
  if (!Number.isFinite(time) || !Number.isFinite(now) || time - now > 60_000) return '';
  if (now - time < 60_000) return t('scheduling.justNow');
  return t('scheduling.ago', { duration: formatSchedulingDuration(at, now, t) });
};

export const formatSchedulingResource = (resource = {}) => {
  const value = resource.value === undefined || resource.value === '' ? '--' : String(resource.value);
  const number = Number(value);
  const memoryGiB = resource.kind === 'memory' && resource.unit === 'MiB'
    && Number.isFinite(number) && number >= 1024 && number % 1024 === 0;
  const amount = memoryGiB ? `${number / 1024} GiB` : `${value}${resource.unit === '%' ? '' : ' '}${resource.unit || ''}`.trim();
  return amount;
};

// Missing defaults and memory percentages stay unknown.
export const getWorkloadRequestTotals = (request) => {
  const resources = request?.resources || [];
  if (resources.some((resource) => !['count', 'core', 'memory', 'memory_percentage'].includes(resource.kind))) {
    return { count: null, cores: null, memoryMiB: null };
  }
  const read = (kind, unit) => {
    const matches = resources.filter((resource) => resource.kind === kind);
    if (matches.length !== 1) return null;
    const resource = matches[0];
    if (unit && resource.unit !== unit) return null;
    if (resource.value === undefined || String(resource.value).trim() === '') return null;
    const value = Number(resource.value);
    return Number.isFinite(value) && value >= 0 ? value : null;
  };
  const requestedCount = read('count');
  const count = Number.isSafeInteger(requestedCount) && requestedCount > 0 ? requestedCount : null;
  const perCardCores = read('core', '%');
  const perCardMemory = read('memory', 'MiB');
  return {
    count,
    cores: count !== null && perCardCores !== null ? count * perCardCores : null,
    // HAMi reads a zero memory request as its default, not zero bytes.
    memoryMiB: count !== null && perCardMemory > 0 ? count * perCardMemory : null,
  };
};

export const schedulingIdentity = (pod = {}) => [pod.namespace, pod.name, pod.uid].join('/');

export const getSchedulingDetailError = (error) => {
  const status = error?.response?.status;
  if (status === 404) return 'gone';
  if (status === 409) return 'recreated';
  return 'loadFailed';
};

export const getSchedulingRequestTiles = (request = {}) => {
  const resources = request.resources || [];
  const find = (...kinds) => resources.find((resource) => kinds.includes(resource.kind));
  const tiles = [];
  if (resources.some((resource) => resource.kind !== 'raw')) {
    const memory = find('memory', 'memory_percentage');
    for (const [kind, resource] of [['count', find('count')], ['core', find('core')], [memory?.kind || 'memory', memory]]) {
      tiles.push({ kind, value: resource ? formatSchedulingResource(resource) : null, ...(resource?.vendor ? { vendor: resource.vendor } : {}) });
    }
  }
  for (const resource of resources.filter((item) => item.kind === 'raw')) {
    tiles.push({ kind: 'raw', name: resource.name, value: formatSchedulingResource(resource) });
  }
  return tiles;
};

const DEVICE_CONSTRAINTS = Object.freeze({
  'nvidia.com/use-gputype': 'useGpuType',
  'nvidia.com/nouse-gputype': 'noUseGpuType',
  'nvidia.com/use-gpuuuid': 'useGpuUuid',
  'nvidia.com/nouse-gpuuuid': 'noUseGpuUuid',
  'hami.io/node-scheduler-policy': 'nodePolicy',
  'hami.io/gpu-scheduler-policy': 'gpuPolicy',
  'nvidia.com/numa-bind': 'numaBind',
});

const DEVICE_LIST_CONSTRAINTS = new Set(['useGpuType', 'noUseGpuType', 'useGpuUuid', 'noUseGpuUuid']);

const parseJSON = (value) => {
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
};

const formatExpression = ({ key = '', operator = '', values = [] } = {}) => (
  Array.isArray(values) && values.length ? `${key} ${operator} ${values.join(', ')}` : `${key} ${operator}`
).trim();

const formatLabelSelector = (selector = {}) => [
  ...Object.entries(selector.matchLabels || {}).map(([key, value]) => `${key}=${value}`),
  ...(selector.matchExpressions || []).map(formatExpression),
];

// Same notation as kubectl describe.
export const formatToleration = ({ key, operator, value, effect, tolerationSeconds } = {}) => {
  let text = `${key || ''}${value ? `=${value}` : ''}${effect ? `:${effect}` : ''}`;
  if (operator === 'Exists' && !value) text = text ? `${text} op=Exists` : 'op=Exists';
  return tolerationSeconds !== undefined && tolerationSeconds !== null ? `${text} for ${tolerationSeconds}s` : text;
};

const describeAffinityTerms = (terms = [], weighted) => terms.map((term) => {
  const podTerm = weighted ? term.podAffinityTerm || {} : term;
  return {
    selector: formatLabelSelector(podTerm.labelSelector),
    topologyKey: podTerm.topologyKey || '',
    namespaces: podTerm.namespaces || [],
    weight: weighted ? term.weight : undefined,
  };
});

export const describeSchedulingConstraints = (constraints = [], gates = []) => {
  const result = {
    nodeSelector: [], device: [], priorityClassName: '', tolerations: [], nodeAffinity: [], podAffinity: [],
    topology: [], other: [], raw: [], gates: [...gates],
  };
  for (const { name = '', value = '' } of constraints) {
    if (name.startsWith('nodeSelector.')) {
      result.nodeSelector.push(`${name.slice('nodeSelector.'.length)}=${value}`);
    } else if (Object.hasOwn(DEVICE_CONSTRAINTS, name)) {
      const key = DEVICE_CONSTRAINTS[name];
      // HAMi reads GPU types and UUIDs as comma-separated lists.
      const values = DEVICE_LIST_CONSTRAINTS.has(key) ? value.split(',').map((item) => item.trim()).filter(Boolean) : [value];
      result.device.push({ key, name, values: values.length ? values : [value] });
    } else if (name === 'priorityClassName') {
      result.priorityClassName = value;
    } else if (['affinity', 'tolerations', 'topologySpreadConstraints'].includes(name)) {
      const parsed = parseJSON(value);
      result.raw.push({ name, text: parsed === undefined ? value : JSON.stringify(parsed, null, 2) });
      if (name === 'tolerations' && Array.isArray(parsed)) {
        result.tolerations = parsed.map(formatToleration);
      } else if (name === 'topologySpreadConstraints' && Array.isArray(parsed)) {
        result.topology = parsed.map((item) => ({
          topologyKey: item.topologyKey || '', maxSkew: item.maxSkew, whenUnsatisfiable: item.whenUnsatisfiable || '',
        }));
      } else if (name === 'affinity' && parsed && typeof parsed === 'object') {
        const node = parsed.nodeAffinity || {};
        const required = node.requiredDuringSchedulingIgnoredDuringExecution?.nodeSelectorTerms || [];
        if (required.length) {
          result.nodeAffinity.push({ mode: 'required', terms: required.map((term) => [
            ...(term.matchExpressions || []), ...(term.matchFields || []),
          ].map(formatExpression)) });
        }
        const preferred = node.preferredDuringSchedulingIgnoredDuringExecution || [];
        if (preferred.length) {
          result.nodeAffinity.push({ mode: 'preferred', terms: preferred.map((item) => [
            ...(item.preference?.matchExpressions || []), ...(item.preference?.matchFields || []),
          ].map(formatExpression)), weights: preferred.map((item) => item.weight) });
        }
        for (const kind of ['podAffinity', 'podAntiAffinity']) {
          const rules = parsed[kind] || {};
          const requiredTerms = describeAffinityTerms(rules.requiredDuringSchedulingIgnoredDuringExecution, false);
          const preferredTerms = describeAffinityTerms(rules.preferredDuringSchedulingIgnoredDuringExecution, true);
          if (requiredTerms.length) result.podAffinity.push({ kind, mode: 'required', terms: requiredTerms });
          if (preferredTerms.length) result.podAffinity.push({ kind, mode: 'preferred', terms: preferredTerms });
        }
      }
    } else {
      result.other.push({ name, value });
    }
  }
  return result;
};

const JSON_TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g;

// Rendered as text nodes, so values never become markup.
export const tokenizeJSON = (text = '') => {
  const segments = [];
  let last = 0;
  for (const match of text.matchAll(JSON_TOKEN)) {
    if (match.index > last) segments.push({ text: text.slice(last, match.index), type: 'plain' });
    if (match[1] !== undefined) {
      segments.push({ text: match[1], type: match[2] ? 'key' : 'string' });
      if (match[2]) segments.push({ text: match[2], type: 'plain' });
    } else {
      segments.push({ text: match[0], type: match[3] ? 'literal' : 'number' });
    }
    last = match.index + match[0].length;
  }
  if (last < text.length) segments.push({ text: text.slice(last), type: 'plain' });
  return segments;
};
