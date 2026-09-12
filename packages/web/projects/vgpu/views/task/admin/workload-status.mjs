export const WORKLOAD_STATUS_CODES = Object.freeze([
  'waiting', 'success', 'not_ready', 'error', 'closed', 'failed', 'terminating', 'unknown',
]);

const STATUS_LABEL_KEYS = Object.freeze({
  waiting: 'statusWaiting',
  success: 'statusRunning',
  not_ready: 'statusNotReady',
  error: 'statusError',
  closed: 'statusCompleted',
  failed: 'statusFailed',
  terminating: 'statusTerminating',
  unknown: 'statusUnknown',
});

const hasText = (value) => typeof value === 'string' && value.trim() !== '';
const hasInteger = (value) => typeof value === 'number' && Number.isInteger(value);

export const getWorkloadStatus = (workload = {}, translate) => {
  const code = WORKLOAD_STATUS_CODES.includes(workload.status) ? workload.status : 'unknown';
  const label = translate(`task.${STATUS_LABEL_KEYS[code]}`);
  const detail = workload.statusDetail;
  if (!detail || typeof detail !== 'object' || Array.isArray(detail) || !Object.keys(detail).length) {
    return { code, label, description: translate('task.statusInfo.legacy', { status: label }) };
  }

  const lines = [translate(`task.statusInfo.summary.${code}`)];
  const add = (key, value) => {
    if (hasText(value) || hasInteger(value)) lines.push(`${translate(`task.statusInfo.${key}`)}: ${value}`);
  };
  const readyText = (ready) => translate(`task.statusInfo.${ready ? 'ready' : 'notReady'}`);
  const containerState = ['Waiting', 'Running', 'Terminated'].includes(detail.containerState)
    ? translate(`task.statusInfo.containerStates.${detail.containerState}`)
    : detail.containerState;
  add('containerState', containerState);
  if (typeof detail.ready === 'boolean') add('containerReady', readyText(detail.ready));
  add('containerReason', detail.reason);
  add('containerMessage', detail.message);
  if (hasInteger(detail.restartCount) && hasText(detail.containerState)) add('restartCount', detail.restartCount);
  if (detail.containerState === 'Terminated' && hasInteger(detail.exitCode)) add('exitCode', detail.exitCode);
  if (detail.restartPending === true) lines.push(translate('task.statusInfo.restartPending'));
  add('lastTerminationReason', detail.lastTerminationReason);
  if (hasInteger(detail.lastExitCode)) add('lastExitCode', detail.lastExitCode);
  add('lastTerminationMessage', detail.lastTerminationMessage);
  add('podPhase', detail.podPhase);
  if (hasText(detail.podReady)) {
    const podReady = detail.podReady === 'True' ? readyText(true)
      : detail.podReady === 'False' ? readyText(false) : translate('task.statusInfo.unknown');
    add('podReady', podReady);
  }
  add('podReadyReason', detail.podReadyReason);
  add('podReadyMessage', detail.podReadyMessage);
  add('podReason', detail.podReason);
  add('podMessage', detail.podMessage);
  if (detail.podReady === 'False') lines.push(translate('task.statusInfo.podReadyScope'));
  return { code, label, description: lines.join('\n') };
};

export const getWorkloadStatusOptions = (translate) => WORKLOAD_STATUS_CODES.map((code) => ({
  value: code,
  label: translate(`task.${STATUS_LABEL_KEYS[code]}`),
}));
