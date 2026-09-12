export const WORKLOAD_STATUS_CODES = Object.freeze([
  'waiting', 'success', 'not_ready', 'error', 'closed', 'failed', 'terminating', 'unknown',
]);

const STATUS_LABEL_KEYS = Object.freeze({
  waiting: 'statusStarting',
  success: 'statusRunning',
  not_ready: 'statusAbnormal',
  error: 'statusAbnormal',
  closed: 'statusCompleted',
  failed: 'statusAbnormal',
  terminating: 'statusTerminating',
  unknown: 'statusUnknown',
});

const hasText = (value) => typeof value === 'string' && value.trim() !== '';
const hasInteger = (value) => typeof value === 'number' && Number.isInteger(value);

export const getWorkloadStatus = (workload = {}, translate) => {
  const code = WORKLOAD_STATUS_CODES.includes(workload.status) ? workload.status : 'unknown';
  const label = translate(`task.${STATUS_LABEL_KEYS[code]}`);
  const detail = workload.statusDetail;
  const needsExplanation = !['success', 'closed'].includes(code);
  if (!detail || typeof detail !== 'object' || Array.isArray(detail) || !Object.keys(detail).length) {
    return {
      code, label, hasDetails: needsExplanation,
      description: translate('task.statusInfo.legacy', { status: label }),
    };
  }

  const lines = [translate(`task.statusInfo.summary.${code}`)];
  const evidence = new Set();
  const add = (key, value) => {
    if (!hasText(value) && !hasInteger(value)) return;
    lines.push(`${translate(`task.statusInfo.${key}`)}: ${value}`);
  };
  const addEvidence = (key, value) => {
    if (!hasText(value) || evidence.has(value)) return;
    evidence.add(value);
    add(key, value);
  };
  const podReadinessDiffers = code === 'success' && ['False', 'Unknown'].includes(detail.podReady);
  const hasRestarts = hasText(detail.containerState) && hasInteger(detail.restartCount) && detail.restartCount > 0;
  const hasPastFailure = hasRestarts && (
    (hasInteger(detail.lastExitCode) && detail.lastExitCode !== 0)
    || (hasText(detail.lastTerminationReason) && detail.lastTerminationReason !== 'Completed')
  );

  // The status already explains normal lifecycle/readiness; add only useful evidence.
  // In particular, Unknown must not repeat a stale Running state as current information.
  if (needsExplanation) {
    addEvidence('containerReason', detail.reason);
    addEvidence('containerMessage', detail.message);
    if (detail.containerState === 'Terminated' && hasInteger(detail.exitCode)) add('exitCode', detail.exitCode);
    if (detail.restartPending === true) lines.push(translate('task.statusInfo.restartPending'));
  }
  if (hasRestarts && (code !== 'closed' || hasPastFailure)) add('restartCount', detail.restartCount);
  if (hasPastFailure) {
    add('lastTerminationReason', detail.lastTerminationReason);
    if (hasInteger(detail.lastExitCode)) add('lastExitCode', detail.lastExitCode);
    add('lastTerminationMessage', detail.lastTerminationMessage);
  }
  if (podReadinessDiffers) {
    lines.push(translate(`task.statusInfo.${detail.podReady === 'False' ? 'podNotReady' : 'podReadinessUnknown'}`));
  }
  if (needsExplanation || podReadinessDiffers) {
    addEvidence('podReadyReason', detail.podReadyReason);
    addEvidence('podReadyMessage', detail.podReadyMessage);
    addEvidence('podReason', detail.podReason);
    addEvidence('podMessage', detail.podMessage);
  }
  return { code, label, hasDetails: needsExplanation || lines.length > 1, description: lines.join('\n') };
};

// Filters describe common workload outcomes; raw states retain precise explanations.
const STATUS_FILTER_LABEL_KEYS = Object.freeze({
  waiting: 'statusStarting',
  success: 'statusRunning',
  abnormal: 'statusAbnormal',
});

export const getWorkloadStatusOptions = (translate) => Object.entries(STATUS_FILTER_LABEL_KEYS).map(([code, labelKey]) => ({
  value: code,
  label: translate(`task.${labelKey}`),
}));
