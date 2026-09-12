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

test('common workload filters offer only starting, running and grouped abnormal states', () => {
  assert.deepEqual(getWorkloadStatusOptions(chinese), [
    { value: 'waiting', label: '启动中' },
    { value: 'success', label: '运行中' },
    { value: 'abnormal', label: '异常' },
  ]);
  assert.deepEqual(getWorkloadStatusOptions(english), [
    { value: 'waiting', label: 'Starting' },
    { value: 'success', label: 'Running' },
    { value: 'abnormal', label: 'Abnormal' },
  ]);
});

test('grouped row labels retain precise raw states and standalone exceptional outcomes', () => {
  const expected = ['启动中', '运行中', '异常', '异常', '已完成', '异常', '终止中', '未知'];
  assert.deepEqual(WORKLOAD_STATUS_CODES.map((status) => getWorkloadStatus({ status }, chinese).label), expected);
  for (const translate of [english, chinese]) {
    for (const status of WORKLOAD_STATUS_CODES) {
      const result = getWorkloadStatus({ status, statusDetail: { reason: 'CrashLoopBackOff' } }, translate);
      assert.equal(result.code, status);
      assert.equal(result.label.includes('CrashLoopBackOff'), false);
      assert.ok(result.description.split('\n').length <= 3);
    }
  }
});

test('not-ready explanation preserves false readiness without repeating status fields', () => {
  const result = getWorkloadStatus({
    status: 'not_ready',
    statusDetail: { containerState: 'Running', ready: false, restartCount: 0, podPhase: 'Running', podReady: 'False' },
  }, english);
  assert.equal(result.label, 'Abnormal');
  assert.equal(result.hasDetails, true);
  assert.equal(result.description, 'Started, but not ready yet.');
});

test('normal exits awaiting restart are not described as completed', () => {
  const result = getWorkloadStatus({
    status: 'waiting',
    statusDetail: { containerState: 'Terminated', ready: false, restartCount: 2, exitCode: 0, restartPending: true },
  }, english);
  assert.equal(result.label, 'Starting');
  assert.equal(result.description, 'The program exited normally and is restarting.\nExit code: 0\nRestart count: 2');
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
  assert.equal(result.description, 'This container is ready, but the overall Pod is not ready.\nLast exit code: 137\nRestart count: 3');
});

test('image pull errors use one conclusion instead of exposing registry and Pod messages', () => {
  const message = 'registry returned <unauthorized>\nplease check credentials';
  const result = getWorkloadStatus({
    status: 'error',
    statusDetail: {
      containerState: 'Waiting', reason: 'ImagePullBackOff', message, restartCount: 0,
      exitCode: 0, podReason: 'Pending', podMessage: 'image not available', lastExitCode: 0,
    },
  }, english);
  assert.equal(result.label, 'Abnormal');
  assert.equal(result.description, 'Image pull failed; retrying.');
});

test('missing container status does not manufacture readiness or restart evidence', () => {
  const result = getWorkloadStatus({
    status: 'waiting', statusDetail: { podPhase: 'Pending', restartCount: 0 },
  }, english);
  assert.equal(result.description, 'Preparing to run.');
});

test('older API responses retain their state without claiming new readiness evidence', () => {
  for (const status of ['success', 'closed', 'failed', 'unknown']) {
    const result = getWorkloadStatus({ status }, english);
    assert.equal(result.code, status);
    assert.equal(result.hasDetails, !['success', 'closed'].includes(status));
    assert.match(result.description, /no further details/);
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
  assert.equal(recovered.description, 'The program recovered after running out of memory.\nLast exit code: 137\nRestart count: 1');
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
  assert.equal(result.description, 'The workload state cannot currently be confirmed.');
});

test('a ready container with unknown Pod readiness keeps useful additional context', () => {
  const result = getWorkloadStatus({
    status: 'success',
    statusDetail: { containerState: 'Running', ready: true, restartCount: 0, podReady: 'Unknown', podReadyReason: 'ReadinessGateUnknown' },
  }, english);
  assert.equal(result.hasDetails, true);
  assert.equal(result.description, 'This container is ready; overall Pod readiness is unknown.');
});

test('crash loops expose the last exit and restart count without repeating error messages', () => {
  const workload = {
    status: 'error',
    statusDetail: {
      containerState: 'Waiting', reason: 'CrashLoopBackOff', restartCount: 5,
      lastTerminationReason: 'Error', lastExitCode: 42,
      message: 'Back-off restarting failed container main in pod webui-demo-crashloop',
      podReadyReason: 'ContainersNotReady', podReadyMessage: 'containers with unready status: [main]',
      lastTerminationMessage: 'duplicated diagnostic message',
    },
  };
  assert.equal(getWorkloadStatus(workload, chinese).description, '程序反复退出，正在重试。\n上次退出码: 42\n重启次数: 5');
  assert.equal(getWorkloadStatus(workload, english).description, 'The program keeps exiting; retrying.\nLast exit code: 42\nRestart count: 5');
  workload.statusDetail.lastExitCode = 0;
  assert.match(getWorkloadStatus(workload, english).description, /Last exit code: 0/);
});

test('terminated errors show the current exit only and distinguish retrying from a final failure', () => {
  const detail = {
    containerState: 'Terminated', reason: 'Error', restartCount: 5, exitCode: 42,
    lastTerminationReason: 'Error', lastExitCode: 42, restartPending: true,
  };
  assert.equal(getWorkloadStatus({ status: 'error', statusDetail: detail }, chinese).description, '程序异常退出，正在重试。\n退出码: 42\n重启次数: 5');
  assert.equal(getWorkloadStatus({ status: 'failed', statusDetail: { ...detail, restartPending: false } }, english).description, 'The program exited with an error and will not restart automatically.\nExit code: 42\nRestart count: 5');
});

test('known startup failures are translated and memory failure never claims GPU memory exhaustion', () => {
  for (const translate of [english, chinese]) {
    for (const reason of ['ImagePullBackOff', 'ErrImagePull', 'InvalidImageName', 'ErrImageNeverPull', 'ImageInspectError', 'CreateContainerConfigError', 'CreateContainerError', 'RunContainerError', 'PreCreateHookError', 'PreStartHookError', 'PostStartHookError']) {
      const result = getWorkloadStatus({ status: 'error', statusDetail: { containerState: 'Waiting', reason, restartCount: 0 } }, translate);
      assert.equal(result.description.split('\n').length, 1);
      assert.equal(result.description.includes(reason), false);
    }
    const result = getWorkloadStatus({
      status: 'error',
      statusDetail: { containerState: 'Waiting', reason: 'CrashLoopBackOff', lastTerminationReason: 'OOMKilled', lastExitCode: 137, restartCount: 2 },
    }, translate);
    assert.equal(result.description.split('\n').length, 3);
    assert.doesNotMatch(result.description, /GPU|显存/);
  }
});

test('unknown failure reasons retain only a short identifier and never raw diagnostic blobs', () => {
  const statusDetail = {
    containerState: 'Waiting', reason: 'VendorStartupError', message: 'a long diagnostic message\n'.repeat(100),
    podReadyMessage: 'duplicated message', restartCount: 0,
  };
  assert.equal(getWorkloadStatus({ status: 'error', statusDetail }, english).description, 'Error reason: VendorStartupError.');
  for (const reason of ['a long diagnostic message\n'.repeat(100), '<unauthorized>']) {
    assert.equal(getWorkloadStatus({ status: 'error', statusDetail: { ...statusDetail, reason } }, english).description, 'The program encountered an error.');
  }
});
