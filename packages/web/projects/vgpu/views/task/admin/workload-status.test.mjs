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
      assert.ok(result.description.includes('CrashLoopBackOff'));
    }
  }
});

test('container readiness and zero-valued evidence remain visible', () => {
  const result = getWorkloadStatus({
    status: 'not_ready',
    statusDetail: { containerState: 'Running', ready: false, restartCount: 0, podPhase: 'Running', podReady: 'False' },
  }, english);
  assert.equal(result.label, 'Not Ready');
  assert.match(result.description, /Container readiness: Not ready/);
  assert.match(result.description, /Restart count: 0/);
  assert.match(result.description, /Pod phase: Running/);
  assert.match(result.description, /Pod readiness: Not ready/);
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
  assert.match(result.description, /Container readiness: Ready/);
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
  assert.match(result.description, /Last exit code: 0/);
  assert.doesNotMatch(result.description, /\nExit code:/);
  assert.match(result.description, /Pod message: image not available/);
});

test('missing container status does not manufacture readiness or restart evidence', () => {
  const result = getWorkloadStatus({
    status: 'waiting', statusDetail: { podPhase: 'Pending', restartCount: 0 },
  }, english);
  assert.match(result.description, /Pod phase: Pending/);
  assert.doesNotMatch(result.description, /Container readiness|Restart count|Exit code/);
});

test('older API responses retain their state without claiming new readiness evidence', () => {
  for (const status of ['success', 'closed', 'failed', 'unknown']) {
    const result = getWorkloadStatus({ status }, english);
    assert.equal(result.code, status);
    assert.match(result.description, /this API does not provide container status details/);
    assert.doesNotMatch(result.description, /Container readiness:|running and ready/);
  }
  assert.equal(getWorkloadStatus({ status: 'new-future-state' }, english).label, 'Unknown');
});
