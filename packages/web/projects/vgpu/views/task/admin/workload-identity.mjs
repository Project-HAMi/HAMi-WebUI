export const createWorkloadRowKey = ({ podUid, name } = {}) =>
  [podUid, name].filter(Boolean).join('/');

export const formatWorkloadName = ({ appName, name } = {}) =>
  [appName, name]
    .filter((value, index, values) => value && values.indexOf(value) === index)
    .join(' / ') || '--';
