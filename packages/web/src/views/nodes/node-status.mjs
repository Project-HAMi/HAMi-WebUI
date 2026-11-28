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

const getTemporarilyUnschedulableStatus = (descriptionKey) => ({
  icon: 'status-unschedulable',
  labelKey: 'node.temporarilyUnschedulable',
  descriptionKey,
  showHelp: true,
});

export const isNodeSchedulingEligible = (node = {}) =>
  !node.isExternal && node.isReady === true && node.isSchedulable === true;

export const getNodeSchedulingEligibilityStatus = (node = {}) => {
  if (node.isExternal) {
    return getTemporarilyUnschedulableStatus('node.schedulingUnknownDescription');
  }

  if (isNodeSchedulingEligible(node)) {
    return {
      icon: 'status-schedulable',
      labelKey: 'node.schedulable',
      showHelp: false,
    };
  }

  if (node.isReady === false && node.isSchedulable === false) {
    return getTemporarilyUnschedulableStatus('node.notInReadyStateAndCordonedDescription');
  }

  if (node.isReady === false) {
    return getTemporarilyUnschedulableStatus('node.notInReadyStateDescription');
  }

  if (node.isSchedulable === false) {
    return getTemporarilyUnschedulableStatus('node.cordonedDescription');
  }

  return getTemporarilyUnschedulableStatus('node.schedulingUnknownDescription');
};

export const matchesNodeSchedulingEligibility = (node, eligibility) => {
  if (!eligibility) return true;
  const schedulable = isNodeSchedulingEligible(node);
  if (eligibility === 'schedulable') return schedulable;
  if (eligibility === 'temporarilyUnschedulable') return !schedulable;
  return true;
};
