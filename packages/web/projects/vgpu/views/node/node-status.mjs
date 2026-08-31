const UNKNOWN_STATUS = Object.freeze({
  icon: 'status-unmanaged',
  labelKey: 'node.unknown',
});

const classifyBooleanStatus = ({ value, isExternal, truthy, falsy }) => {
  if (isExternal || value === undefined || value === null) {
    return UNKNOWN_STATUS;
  }
  return value ? truthy : falsy;
};

export const getNodeReadinessStatus = ({ isReady, isExternal } = {}) =>
  classifyBooleanStatus({
    value: isReady,
    isExternal,
    truthy: {
      icon: 'status-schedulable',
      labelKey: 'node.ready',
    },
    falsy: {
      icon: 'status-unschedulable',
      labelKey: 'node.notReady',
    },
  });

export const getNodeSchedulingStatus = ({ isSchedulable, isExternal } = {}) =>
  classifyBooleanStatus({
    value: isSchedulable,
    isExternal,
    truthy: {
      icon: 'status-schedulable',
      labelKey: 'node.schedulingEnabled',
    },
    falsy: {
      icon: 'status-unschedulable',
      labelKey: 'node.schedulingDisabled',
    },
  });
