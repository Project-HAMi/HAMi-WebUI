export const createWorkloadRowKey = ({ podUid, name } = {}) =>
  [podUid, name].filter(Boolean).join('/');

export const formatWorkloadName = ({ appName, name } = {}) =>
  [appName, name]
    .filter((value, index, values) => value && values.indexOf(value) === index)
    .join(' / ') || '--';

export const parseWorkloadMetricIdentity = (metricName) => {
  if (typeof metricName !== 'string') return null;
  const parts = metricName.split(':');
  if (parts.length !== 2 || !parts.every((part) => part.trim())) return null;
  return { name: parts[0], podUid: parts[1] };
};

export const buildWorkloadIdentityIndex = (workloads = []) =>
  new Map(workloads
    .filter((workload) => workload?.podUid && workload?.name)
    .map((workload) => [createWorkloadRowKey(workload), workload]));

export const resolveWorkloadRankingIdentity = (metricName, workloadsByIdentity) => {
  const identity = parseWorkloadMetricIdentity(metricName);
  const workload = identity && workloadsByIdentity?.get(createWorkloadRowKey(identity));
  return {
    metricName,
    identity,
    appName: workload?.appName || '',
    namespace: workload?.namespace || workload?.namespaceName || '',
  };
};

export const createWorkloadDetailLocation = (metricName) => {
  const identity = parseWorkloadMetricIdentity(metricName);
  return identity ? {
    path: '/admin/vgpu/task/admin/detail',
    query: identity,
  } : null;
};
