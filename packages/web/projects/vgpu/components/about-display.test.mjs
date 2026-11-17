import assert from 'node:assert/strict';
import test from 'node:test';

import en from '../../../src/locales/en.js';
import zh from '../../../src/locales/zh.js';
import {
  describeWebUIBuild,
  displayVersion,
  docsURL,
  issueEnvironmentSummary,
  kubeletVersions,
  refreshAge,
  summarizeHami,
} from './about-display.mjs';

const lookup = (messages, key) => key.split('.').reduce((value, part) => value?.[part], messages);

const plugin = (index) => ({
  job: 'monitor',
  instance: `10.0.0.${index}:9394`,
  pod: `hami-device-plugin-${index}`,
  component: 'hami-device-plugin',
  podUid: `uid-${index}`,
});

// Shape of GET /v1/about.
const about = {
  webui: { version: 'v2.0.0', commit: '4f5875c0d1a3e7f1f9b1c1b5a9d0e0c7b6a5f4e3' },
  kubernetesVersion: 'v1.31.2',
  hami: {
    state: 'ok',
    supported: ['2.10', '2.11'],
    versions: [
      {
        version: 'v2.10.0',
        support: 'supported',
        targets: [{ job: 'hami-scheduler', instance: '10.0.0.2:31993', component: 'hami-scheduler' }, plugin(3), plugin(4)],
      },
      { version: 'v2.9.0', support: 'unsupported', targets: [plugin(9)] },
    ],
    components: [
      { name: 'hami-device-plugin', namespace: 'kube-system', pods: 3, ready: 2, nodes: 3, restarts: 4 },
      { name: 'hami-scheduler', namespace: 'kube-system', pods: 1, ready: 1, nodes: 1, restarts: 0 },
    ],
  },
  kubelets: [{ version: 'v1.30.10', nodes: 2 }, { version: 'v1.29.4', nodes: 1 }],
  prometheus: { reachable: true, metricsRefreshedAt: '2026-10-25T15:48:30Z', version: '3.14.0' },
  checkedAt: '2026-10-25T15:49:00Z',
};

test('WebUI builds are labeled by how they were built', () => {
  assert.deepEqual(describeWebUIBuild(about.webui), {
    kind: 'release',
    version: 'v2.0.0',
    commit: '4f5875c',
    commitURL: 'https://github.com/Project-HAMi/HAMi-WebUI/commit/4f5875c0d1a3e7f1f9b1c1b5a9d0e0c7b6a5f4e3',
    label: 'v2.0.0',
  });
  assert.equal(describeWebUIBuild({ version: '2.0.1' }).version, 'v2.0.1');
  const main = describeWebUIBuild({ version: 'main', commit: '4f5875c0d1' });
  assert.deepEqual([main.label, main.kind], ['main (4f5875c)', 'development']);
  assert.deepEqual([describeWebUIBuild({ version: 'dev', commit: '' }).label, describeWebUIBuild({ version: '' }).kind], ['dev', 'local']);
  assert.equal(describeWebUIBuild({ version: 'dev', commit: 'not a sha' }).commitURL, '');
  assert.equal(describeWebUIBuild(undefined).kind, 'unknown');
});

test('each HAMi component carries its Pods and the versions its targets report', () => {
  const hami = summarizeHami(about.hami);
  const [scheduler, devicePlugin] = hami.components;
  assert.deepEqual(scheduler, {
    name: 'hami-scheduler',
    namespaces: ['kube-system'],
    pods: 1,
    ready: 1,
    restarts: 0,
    versions: [{ version: 'v2.10.0', label: 'v2.10.0', detailLabel: 'v2.10.0', key: '["v2.10.0",""]', support: 'supported', count: 1, unit: 'targets', jobs: ['hami-scheduler'], names: [], moreNames: 0 }],
  });
  assert.deepEqual([devicePlugin.ready, devicePlugin.pods], [2, 3]);
  assert.deepEqual(devicePlugin.versions.map(({ version, count, unit, names }) => [version, count, unit, names]), [
    ['v2.10.0', 2, 'pods', []],
    ['v2.9.0', 1, 'pods', ['hami-device-plugin-9']],
  ]);
  assert.deepEqual(hami.notices, ['mixed', 'unsupported']);
});

test('mixed builds retain their revisions in details and issue reports', () => {
  const hami = { state: 'ok', versions: ['aaaaaaa1', 'bbbbbbb2'].map((revision, i) => ({
    version: 'v2.11.0', revision, targets: [plugin(i)],
  })) };
  const builds = () => summarizeHami(hami).components[1].versions;
  assert.deepEqual(builds().map(entry => entry.detailLabel), ['v2.11.0 (aaaaaaa)', 'v2.11.0 (bbbbbbb)']);
  assert.notEqual(builds()[0].key, builds()[1].key);
  assert.match(issueEnvironmentSummary({ hami }), /v2\.11\.0 \(bbbbbbb\)/);
  hami.versions[1].revision = 'aaaaaaa2';
  assert.deepEqual(builds().map(entry => entry.detailLabel), ['v2.11.0 (aaaaaaa1)', 'v2.11.0 (aaaaaaa2)']);
});

test('Pods are counted by the matched UID, never by bare name', () => {
  const twoJobs = summarizeHami({
    state: 'ok',
    versions: [{ version: 'v2.10.0', support: 'supported', targets: [plugin(1), { ...plugin(1), job: 'second-job' }, plugin(2)] }],
  });
  assert.deepEqual(twoJobs.components[1].versions.map(({ count, unit, jobs }) => [count, unit, jobs]), [[2, 'pods', ['monitor', 'second-job']]]);
  const sameName = summarizeHami({
    state: 'ok',
    versions: [{ version: 'v2.10.0', targets: [{ ...plugin(0), podUid: 'ns-a' }, { ...plugin(0), podUid: 'ns-b' }] }],
  });
  assert.deepEqual(sameName.components[1].versions.map(({ count, unit }) => [count, unit]), [[2, 'pods']]);
  const unmatched = summarizeHami({ state: 'ok', versions: [{ version: 'v2.10.0', targets: [{ ...plugin(1), podUid: '' }, plugin(2)] }] });
  assert.deepEqual(unmatched.components[1].versions.map(({ count, unit }) => [count, unit]), [[2, 'targets']]);
});

test('targets that match no component Pod are kept apart', () => {
  const hami = summarizeHami({
    state: 'ok',
    versions: [{ version: 'v2.10.0', support: 'supported', targets: [{ job: 'accelerator-control', instance: '10.0.0.7:31993', component: '' }] }],
  });
  assert.deepEqual(hami.components.map(({ versions }) => versions.length), [0, 0]);
  assert.deepEqual(hami.unattributed.map(({ version, count, unit, jobs }) => [version, count, unit, jobs]), [['v2.10.0', 1, 'targets', ['accelerator-control']]]);
  assert.match(issueEnvironmentSummary({ hami: { state: 'ok', versions: [{ version: 'v2.10.0', targets: [{ job: 'x', instance: '10.0.0.7:1' }] }] } }),
    /- HAMi, unattributed: v2\.10\.0 \(1 scrape target\)(?:\n|$)/);
});

test('a component without Pods still appears with observed zero counts', () => {
  const hami = summarizeHami({ state: 'missing', versions: [], components: [] });
  assert.deepEqual(hami.components.map(({ name, pods, ready, versions }) => [name, pods, ready, versions]), [
    ['hami-scheduler', 0, 0, []],
    ['hami-device-plugin', 0, 0, []],
  ]);
  assert.deepEqual(hami.notices, ['missing']);
  assert.deepEqual(summarizeHami(undefined).notices, ['unavailable']);
});

test('development builds are notices, and long labels fall back to the revision', () => {
  const hami = summarizeHami({
    state: 'ok',
    versions: [{ version: 'f4079c0-f4079c08e9380acc3012b8af0caa7d040819a7e2', revision: 'f4079c0', support: 'development', targets: [{ job: 'hami-scheduler', component: 'hami-scheduler' }] }],
  });
  assert.deepEqual([hami.notices, hami.components[0].versions[0].label], [['development'], 'f4079c0']);
});

test('only the first three Pods of a minority version are named', () => {
  const targets = Array.from({ length: 7 }, (_, index) => plugin(index));
  const hami = summarizeHami({ state: 'ok', versions: [{ version: 'v2.10.0', targets: targets.concat(plugin(20)) }, { version: 'v2.11.0', targets }] });
  const minority = hami.components[1].versions[1];
  assert.deepEqual([minority.names.length, minority.moreNames], [3, 4]);
});

test('refresh age uses the server clock', () => {
  assert.deepEqual(refreshAge(about.prometheus.metricsRefreshedAt, about.checkedAt), { unit: 'seconds', count: 30 });
  assert.deepEqual(refreshAge('2026-10-25T12:00:00Z', about.checkedAt), { unit: 'hours', count: 3 });
  assert.equal(refreshAge('', about.checkedAt), undefined);
});

test('kubelet versions skip entries without a version', () => {
  assert.deepEqual(kubeletVersions({ kubelets: [{ version: '', nodes: 1 }, { version: 'v1.30.10', nodes: '3' }] }), [{ version: 'v1.30.10', nodes: 3 }]);
});

test('bare version numbers get a v prefix', () => {
  assert.deepEqual(['3.14.0', 'v1.31.0', 'main', ''].map(displayVersion), ['v3.14.0', 'v1.31.0', 'main', '']);
});

test('the issue link carries versions and counts but no Pod names', () => {
  const body = issueEnvironmentSummary(about);
  assert.match(body, /- HAMi-WebUI: v2\.0\.0\n/);
  assert.match(body, /- Kubernetes: v1\.31\.2; kubelet v1\.30\.10 ×2, v1\.29\.4 ×1\n/);
  assert.match(body, /- Prometheus: v3\.14\.0, reachable\n/);
  assert.match(body, /- HAMi scheduler: v2\.10\.0 \(1 scrape target\); found 1 Pod, 1 ready\n/);
  assert.match(body, /- HAMi device plugin: v2\.10\.0 \(2 Pods\), v2\.9\.0 \(1 Pod\); found 3 Pods, 2 ready(?:\n|$)/);
  assert.doesNotMatch(body, /unattributed/);
  assert.doesNotMatch(body, /hami-device-plugin-\d|10\.0\.0/);
  assert.equal(issueEnvironmentSummary(null), '');
});

test('docs follow the page language', () => {
  assert.equal(docsURL('zh'), 'https://project-hami.io/zh/docs/userguide/hami-webui-user-guide');
  assert.equal(docsURL('ja'), 'https://project-hami.io/docs/userguide/hami-webui-user-guide');
});

test('every state, notice and edge has copy in both languages', () => {
  const keys = [
    ...['development', 'local', 'unknown'].map((kind) => `about.build.${kind}`),
    ...['missing', 'unavailable', 'mixed', 'unsupported', 'development'].map((notice) => `about.notice.${notice}`),
    ...['seconds', 'minutes', 'hours', 'days'].map((unit) => `about.age.${unit}`),
    ...['resources', 'metrics', 'scheduling', 'registration', 'usage'].map((edge) => `about.topology.${edge}`),
    ...['scheduler', 'devicePlugin'].map((name) => `about.component.${name}`),
    ...['ready', 'podCountsHint', 'versionPods', 'versionTargets', 'unattributed', 'unattributedItem', 'refreshFailed', 'notRefreshed'].map((key) => `about.${key}`),
  ];
  for (const messages of [en, zh]) {
    for (const key of keys) assert.equal(typeof lookup(messages, key), 'string', key);
  }
});
