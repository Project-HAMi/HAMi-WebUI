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
const isReasonCode = (value) => typeof value === 'string' && /^[A-Za-z][A-Za-z0-9_.-]{0,63}$/.test(value);

const REASON_SUMMARY_KEYS = Object.freeze({
  ImagePullBackOff: 'imagePull',
  ErrImagePull: 'imagePull',
  InvalidImageName: 'invalidImage',
  ErrImageNeverPull: 'missingLocalImage',
  ImageInspectError: 'imageInspect',
  CreateContainerConfigError: 'configuration',
  CreateContainerError: 'creation',
  RunContainerError: 'start',
  PreCreateHookError: 'start',
  PreStartHookError: 'start',
  PostStartHookError: 'start',
});

const getSummary = (code, detail, hasPastFailure, translate) => {
  const summary = (key) => translate(`task.statusInfo.${key}`);
  // Current state wins over stale termination/readiness evidence.
  if (code === 'unknown' || code === 'terminating' || code === 'closed' || code === 'not_ready') {
    return summary(`summary.${code}`);
  }
  if (code === 'success') {
    if (detail.podReady === 'False') return summary('podNotReady');
    if (detail.podReady === 'Unknown') return summary('podReadinessUnknown');
    if (hasPastFailure) return summary(detail.lastTerminationReason === 'OOMKilled' ? 'recoveredOom' : 'recoveredFailure');
    return summary('summary.success');
  }
  if (code === 'waiting') {
    return summary(detail.restartPending === true ? 'normalRestart' : 'summary.waiting');
  }
  if (detail.reason === 'CrashLoopBackOff') {
    return summary(detail.lastTerminationReason === 'OOMKilled' ? 'reason.oomRetry' : 'reason.crashLoop');
  }
  if (Object.hasOwn(REASON_SUMMARY_KEYS, detail.reason)) return summary(`reason.${REASON_SUMMARY_KEYS[detail.reason]}`);
  if (detail.reason === 'OOMKilled') return summary(detail.restartPending === true ? 'reason.oomRetry' : 'reason.oom');
  if (detail.containerState === 'Terminated' && detail.restartPending === true) return summary('reason.exitRetry');
  if (isReasonCode(detail.reason) && detail.reason !== 'Error') {
    return translate('task.statusInfo.reasonCode', { reason: detail.reason });
  }
  return summary(`summary.${code}`);
};

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

  const podReadinessDiffers = code === 'success' && ['False', 'Unknown'].includes(detail.podReady);
  const hasRestarts = hasText(detail.containerState) && hasInteger(detail.restartCount) && detail.restartCount > 0;
  const hasPastFailure = hasRestarts && (
    (hasInteger(detail.lastExitCode) && detail.lastExitCode !== 0)
    || (hasText(detail.lastTerminationReason) && detail.lastTerminationReason !== 'Completed')
  );

  const lines = [getSummary(code, detail, hasPastFailure, translate)];
  const addNumber = (key, value) => lines.push(`${translate(`task.statusInfo.${key}`)}: ${value}`);
  // One conclusion and at most two useful numbers; full evidence remains in the API.
  if (!['unknown', 'terminating', 'closed'].includes(code)) {
    if (detail.containerState === 'Terminated' && hasInteger(detail.exitCode)) {
      addNumber('exitCode', detail.exitCode);
    } else if ((detail.reason === 'CrashLoopBackOff' || hasPastFailure) && hasInteger(detail.lastExitCode)) {
      addNumber('lastExitCode', detail.lastExitCode);
    }
    if (hasRestarts) addNumber('restartCount', detail.restartCount);
  }
  return { code, label, hasDetails: needsExplanation || podReadinessDiffers || lines.length > 1, description: lines.join('\n') };
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
