import assert from 'node:assert/strict';
import test from 'node:test';
import en from '../../../../../src/locales/en.js';
import zh from '../../../../../src/locales/zh.js';
import { getWorkloadStatus, getWorkloadStatusOptions, WORKLOAD_STATUS_CODES } from './workload-status.mjs';

const translator = (messages) => (key, params = {}) => {
  const value = key.split('.').reduce((entry, segment) => entry?.[segment], messages);
  assert.equal(typeof value, 'string', `Missing translation: ${key}`);
  return value.replace(/\{(\w+)\}/g, (_, name) => params[name]);
};
const english = translator(en);
const chinese = translator(zh);

test('all workload states have standalone labels and compatible filter values in both languages', () => {
  const expected = ['等待中', '运行中', '未就绪', '异常', '已完成', '失败', '终止中', '未知'];
  assert.deepEqual(getWorkloadStatusOptions(chinese).map((item) => item.label), expected);
  assert.deepEqual(getWorkloadStatusOptions(english).map((item) => item.value), WORKLOAD_STATUS_CODES);
  for (const translate of [english, chinese]) {
    for (const status of WORKLOAD_STATUS_CODES) {
      const result = getWorkloadStatus({ status, statusDetail: { reason: 'CrashLoopBackOff' } }, translate);
      assert.equal(result.code, status);
      assert.equal(result.label.includes('CrashLoopBackOff'), false);
      if (!['success', 'closed'].includes(status)) assert.ok(result.description.includes('CrashLoopBackOff'));
    }
  }
});

test('not-ready explanation preserves false readiness without repeating status fields', () => {
  const result = getWorkloadStatus({
    status: 'not_ready',
    statusDetail: { containerState: 'Running', ready: false, restartCount: 0, podPhase: 'Running', podReady: 'False' },
  }, english);
  assert.equal(result.label, 'Not Ready');
  assert.equal(result.hasDetails, true);
  assert.match(result.description, /running, but Kubernetes has not marked it ready/);
  assert.doesNotMatch(result.description, /Container readiness:|Restart count:|Pod phase:|Pod readiness:/);
  assert.doesNotMatch(result.description, /Exit code:/);
});

test('normal exits awaiting restart are not described as completed', () => {
  const result = getWorkloadStatus({
    status: 'waiting',
    statusDetail: { containerState: 'Terminated', ready: false, restartCount: 2, exitCode: 0, restartPending: true },
  }, english);
  assert.equal(result.label, 'Waiting');
  assert.match(result.description, /Exit code: 0/);
  assert.match(result.description, /will restart according to the Pod restart policy/);
  assert.doesNotMatch(result.description, /not awaiting/);
});

test('Pod readiness and previous termination do not override a recovered container', () => {
  const result = getWorkloadStatus({
    status: 'success',
    statusDetail: {
      containerState: 'Running', ready: true, restartCount: 3,
      podReady: 'False', podReadyReason: 'ContainersNotReady', podReadyMessage: 'sidecar is not ready',
      lastTerminationReason: 'OOMKilled', lastExitCode: 137, lastTerminationMessage: 'previous failure',
    },
  }, english);
  assert.equal(result.label, 'Running');
  assert.equal(result.hasDetails, true);
  assert.match(result.description, /container is running and ready/);
  assert.match(result.description, /Last termination reason: OOMKilled/);
  assert.match(result.description, /Last exit code: 137/);
  assert.match(result.description, /sidecar is not ready/);
  assert.match(result.description, /does not mean this container is in error/);
});

test('raw reasons and messages remain plain evidence with applicable exit codes', () => {
  const message = 'registry returned <unauthorized>\nplease check credentials';
  const result = getWorkloadStatus({
    status: 'error',
    statusDetail: {
      containerState: 'Waiting', reason: 'ImagePullBackOff', message, restartCount: 0,
      exitCode: 0, podReason: 'Pending', podMessage: 'image not available', lastExitCode: 0,
    },
  }, english);
  assert.equal(result.label, 'Error');
  assert.ok(result.description.includes(message));
  assert.doesNotMatch(result.description, /Last exit code:|Restart count:/);
  assert.doesNotMatch(result.description, /\nExit code:/);
  assert.match(result.description, /Pod message: image not available/);
});

test('missing container status does not manufacture readiness or restart evidence', () => {
  const result = getWorkloadStatus({
    status: 'waiting', statusDetail: { podPhase: 'Pending', restartCount: 0 },
  }, english);
  assert.match(result.description, /waiting to start or restart/);
  assert.doesNotMatch(result.description, /Container readiness|Restart count|Exit code/);
});

test('older API responses retain their state without claiming new readiness evidence', () => {
  for (const status of ['success', 'closed', 'failed', 'unknown']) {
    const result = getWorkloadStatus({ status }, english);
    assert.equal(result.code, status);
    assert.equal(result.hasDetails, !['success', 'closed'].includes(status));
    assert.match(result.description, /this API does not provide container status details/);
    assert.doesNotMatch(result.description, /Container readiness:|running and ready/);
  }
  assert.equal(getWorkloadStatus({ status: 'new-future-state' }, english).label, 'Unknown');
});

test('healthy running and ordinary completed workloads need no redundant help', () => {
  for (const translate of [english, chinese]) {
    for (const podReady of ['True', '']) {
      const result = getWorkloadStatus({
        status: 'success',
        statusDetail: { containerState: 'Running', ready: true, restartCount: 0, podPhase: 'Running', podReady },
      }, translate);
      assert.equal(result.hasDetails, false);
      assert.equal(result.description, translate('task.statusInfo.summary.success'));
    }
    const completed = getWorkloadStatus({
      status: 'closed',
      statusDetail: { containerState: 'Terminated', reason: 'Completed', exitCode: 0, restartCount: 0, podPhase: 'Succeeded' },
    }, translate);
    assert.equal(completed.hasDetails, false);
    assert.equal(completed.description, translate('task.statusInfo.summary.closed'));
  }
});

test('recovered failures are historical and require meaningful observed restart evidence', () => {
  const detail = { containerState: 'Running', ready: true, podReady: 'True', lastTerminationReason: 'OOMKilled', lastExitCode: 137 };
  const unobserved = getWorkloadStatus({ status: 'success', statusDetail: { ...detail, restartCount: 0 } }, english);
  assert.equal(unobserved.hasDetails, false);
  assert.doesNotMatch(unobserved.description, /OOMKilled|137/);
  const recovered = getWorkloadStatus({ status: 'success', statusDetail: { ...detail, restartCount: 1 } }, english);
  assert.equal(recovered.hasDetails, true);
  assert.equal(recovered.label, 'Running');
  assert.match(recovered.description, /Restart count: 1\nLast termination reason: OOMKilled\nLast exit code: 137/);
  const normalExit = getWorkloadStatus({
    status: 'success',
    statusDetail: { ...detail, restartCount: 1, lastTerminationReason: 'Completed', lastExitCode: 0 },
  }, english);
  assert.doesNotMatch(normalExit.description, /Last termination|Last exit/);
});

test('unknown status describes lost information without presenting stale running state', () => {
  const result = getWorkloadStatus({
    status: 'unknown',
    statusDetail: { containerState: 'Running', ready: true, podPhase: 'Running', podReady: 'Unknown', podReadyReason: 'NodeStatusUnknown', podReadyMessage: 'Kubelet stopped posting node status.' },
  }, english);
  assert.equal(result.hasDetails, true);
  assert.match(result.description, /not enough information/);
  assert.match(result.description, /NodeStatusUnknown/);
  assert.match(result.description, /Kubelet stopped posting node status/);
  assert.doesNotMatch(result.description, /running|Running|readiness: Ready/);
});

test('a ready container with unknown Pod readiness keeps useful additional context', () => {
  const result = getWorkloadStatus({
    status: 'success',
    statusDetail: { containerState: 'Running', ready: true, restartCount: 0, podReady: 'Unknown', podReadyReason: 'ReadinessGateUnknown' },
  }, english);
  assert.equal(result.hasDetails, true);
  assert.match(result.description, /readiness of the entire Pod is unknown/);
  assert.match(result.description, /ReadinessGateUnknown/);
  assert.doesNotMatch(result.description, /Restart count: 0/);
});
