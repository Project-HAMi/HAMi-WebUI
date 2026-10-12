import assert from 'node:assert/strict';
import test from 'node:test';
import english from '../../../../../src/locales/en.js';
import chinese from '../../../../../src/locales/zh.js';
import {
  describeSchedulingConstraints, formatSchedulingAgo, formatSchedulingDuration, formatSchedulingResource, formatSchedulingWait, formatToleration, getSchedulingChecks,
  getSchedulingDetailError, getSchedulingReasons, getSchedulingRequestTiles, getWorkloadRequestTotals, getSchedulingSummary, schedulingIdentity, tokenizeJSON,
} from './scheduling-display.mjs';

const en = english.scheduling;
const zh = chinese.scheduling;
let language = zh;
const t = (key, params = {}) => {
  const text = key.split('.').slice(1).reduce((value, part) => value?.[part], language);
  assert.equal(typeof text, 'string', `Missing translation: ${key}`);
  return text.replace(/\{(\w+)\}/g, (_, name) => params[name]);
};

test('bound Pod clears historical capacity reasons and suggested capacity actions', () => {
  const pod = { nodeName: 'node-a', stage: 'waiting', reasonCodes: ['CardInsufficientMemory'], allocatedContainers: ['main'] };
  assert.equal(getSchedulingSummary(pod, t), zh.boundSummary);
  assert.deepEqual(getSchedulingChecks(pod, t), []);
});

test('node assignment without the selected container allocation is not successful HAMi allocation', () => {
  const pod = { nodeName: 'node-a', stage: 'bound', schedulerName: 'default-scheduler', allocatedContainers: ['other'], reasonCodes: ['CardInsufficientMemory'] };
  assert.equal(getSchedulingSummary(pod, t, 'main'), zh.noAllocation);
  assert.deepEqual(getSchedulingChecks(pod, t, 'main'), [zh.check.allocation]);
  assert.equal(getSchedulingSummary(pod, t, 'other'), zh.boundSummary);
  assert.deepEqual(getSchedulingChecks(pod, t, 'other'), []);
  assert.equal(getSchedulingSummary({ stage: 'waiting', reasonCodes: ['PVCNotFound'] }, t), zh.reason.pvcMissing);
});

test('gates, terminal and incomplete assignment never become GPU shortage conclusions', () => {
  for (const stage of ['gated', 'terminating', 'finished', 'unknown']) {
    assert.equal(getSchedulingSummary({ stage, reasonCodes: ['CardInsufficientMemory'] }, t), zh.stageSummary[stage]);
  }
  assert.equal(getSchedulingSummary({ stage: 'finished', nodeName: 'node-a' }, t), zh.stageSummary.finished);
  assert.equal(getSchedulingSummary({ stage: 'terminating', nodeName: 'node-a' }, t), zh.stageSummary.terminating);
});

test('known GPU and host memory constraints remain distinct in both languages', () => {
  for (const locale of ['zh', 'en']) {
    language = locale === 'zh' ? zh : en;
    const gpu = getSchedulingSummary({ reasonCodes: ['CardInsufficientMemory'] }, t);
    const host = getSchedulingSummary({ reasonCodes: ['InsufficientHostMemory'] }, t);
    assert.notEqual(gpu, host);
    assert.match(gpu, /GPU/);
    assert.match(host, locale === 'zh' ? /主机内存/ : /host memory/);
  }
  language = zh;
});

test('compute and memory shortages lead to specific per-GPU allocation checks', () => {
  for (const messages of [zh, en]) {
    language = messages;
    const compute = { stage: 'waiting', reasonCodes: ['CardInsufficientCore'] };
    const memory = { stage: 'waiting', reasonCodes: ['CardInsufficientMemory'] };
    assert.equal(getSchedulingSummary(compute, t), messages.reason.core);
    assert.equal(getSchedulingSummary(memory, t), messages.reason.memory);
    assert.deepEqual(getSchedulingChecks(compute, t), [messages.check.compute]);
    assert.deepEqual(getSchedulingChecks(memory, t), [messages.check.memory]);
    assert.deepEqual(getSchedulingChecks({ stage: 'waiting', reasonCodes: ['CardComputeUnitsExhausted'] }, t), [messages.check.compute]);
  }
  language = zh;
});

test('paused scheduling directs the creator to prerequisites instead of old capacity feedback', () => {
  const pod = { stage: 'gated', reasonCodes: ['CardInsufficientMemory'], schedulerName: 'default-scheduler' };
  assert.equal(getSchedulingSummary(pod, t), '调度已暂缓');
  assert.deepEqual(getSchedulingChecks(pod, t), [zh.check.gates]);
  assert.doesNotMatch(getSchedulingChecks(pod, t)[0], /删除|解除|控制器/);
});

test('missing feedback points to the scheduler while unrecognized feedback points to the original records', () => {
  for (const schedulerName of ['default-scheduler', 'hami-scheduler', 'custom-profile']) {
    for (const [reasonCodes, check] of [
      [[], zh.check.scheduler],
      [['NoFeedback'], zh.check.scheduler],
      [['UnknownSchedulingReason'], zh.check.rawFeedback],
      [['FutureReason'], zh.check.rawFeedback],
    ]) {
      const pod = { stage: 'waiting', schedulerName, reasonCodes, reasonSource: 'unknown' };
      assert.deepEqual(getSchedulingChecks(pod, t), [check]);
      assert.doesNotMatch(getSchedulingSummary(pod, t), /未接入|不使用 HAMi/);
    }
  }
  assert.deepEqual(getSchedulingChecks({ stage: 'waiting', reasonCodes: ['NodeAffinity', 'NoFeedback'], schedulerName: 'default-scheduler' }, t), [zh.check.node]);
  assert.deepEqual(getSchedulingChecks({ stage: 'waiting', reasonCodes: ['NoFeedback'], reasonSource: 'hami' }, t), []);
  for (const stage of ['bound', 'finished', 'terminating', 'unknown']) {
    assert.deepEqual(getSchedulingChecks({ stage, reasonCodes: ['NoFeedback'] }, t), []);
  }
});

test('Kubernetes extended resource failure stays distinct from HAMi compute or GPU memory shortage', () => {
  const pod = { stage: 'waiting', reasonCodes: ['ExtendedResourceUnavailable'], reasonSource: 'kubernetes' };
  for (const messages of [zh, en]) {
    language = messages;
    const summary = getSchedulingSummary(pod, t);
    assert.equal(summary, messages.reason.extendedResource);
    assert.notEqual(summary, messages.reason.core);
    assert.notEqual(summary, messages.reason.memory);
    assert.notEqual(summary, messages.reason.hostMemory);
    assert.deepEqual(getSchedulingChecks(pod, t), [messages.check.extendedResource]);
  }
  language = zh;
});

test('multiple constraints are summarized without repeating aliases or exposing raw error blobs', () => {
  assert.equal(getSchedulingSummary({ reasonCodes: ['NodeInsufficientDevice', 'AllocatedCardsInsufficientRequest'] }, t), zh.reason.devices);
  assert.equal(getSchedulingSummary({ reasonCodes: ['CardInsufficientMemory', 'NodeAffinity'] }, t), zh.multipleReasons);
  assert.equal(getSchedulingSummary({ reasonCodes: ['unrecognized'], condition: { message: '<script>alert(1)</script>' } }, t), zh.reason.unknown);
  assert.equal(getSchedulingReasons({ reasonCodes: ['NoFeedback'] }, t)[0].label, zh.reason.noFeedback);
});

test('container requests use the same total resource basis as allocated rows', () => {
  const request = { container: 'main', containerKind: 'regular', resources: [
    { name: 'nvidia.com/gpu', kind: 'count', value: '2' },
    { name: 'nvidia.com/gpucores', kind: 'core', value: '20', unit: '%' },
    { name: 'nvidia.com/gpumem', kind: 'memory', value: '8192', unit: 'MiB' },
  ] };
  const expected = { count: 2, cores: 40, memoryMiB: 16384 };
  assert.deepEqual(getWorkloadRequestTotals(request), expected);
  assert.deepEqual(getWorkloadRequestTotals({ ...request, containerKind: 'init' }), expected);
  assert.deepEqual(getWorkloadRequestTotals({ ...request, containerKind: 'sidecar' }), expected);
  assert.equal(formatSchedulingResource({ kind: 'memory', value: '1536', unit: 'MiB' }), '1536 MiB');
  assert.equal(formatSchedulingResource({ kind: 'core', value: '0', unit: '%' }), '0%');
});

test('request totals never invent count, defaults, physical memory or vendor units', () => {
  const count = { kind: 'count', value: '2' };
  const core = { kind: 'core', value: '0', unit: '%' };
  const memoryPercentage = { kind: 'memory_percentage', value: '50', unit: '%' };
  const totals = (resources) => getWorkloadRequestTotals({ resources });
  const unknown = { count: null, cores: null, memoryMiB: null };
  assert.deepEqual(getWorkloadRequestTotals(null), unknown);
  assert.deepEqual(totals([core]), unknown);
  assert.deepEqual(totals([{ ...count, value: '0' }, core]), unknown);
  assert.deepEqual(totals([{ ...count, value: '1.5' }, core]), unknown);
  assert.deepEqual(totals([count]), { count: 2, cores: null, memoryMiB: null });
  assert.deepEqual(totals([count, core, memoryPercentage]), { count: 2, cores: 0, memoryMiB: null });
  assert.deepEqual(totals([count, { kind: 'memory', value: '0', unit: 'MiB' }]), { count: 2, cores: null, memoryMiB: null });
  assert.deepEqual(totals([count, memoryPercentage, { kind: 'memory', value: '512', unit: 'MiB' }]), { count: 2, cores: null, memoryMiB: 1024 });
  assert.deepEqual(totals([count, core, { name: 'vendor/card', value: '1', kind: 'raw' }]), unknown);
});

test('stable identity separates same-name Pod replacement and errors preserve that boundary', () => {
  assert.notEqual(schedulingIdentity({ namespace: 'dev', name: 'app', uid: 'old' }), schedulingIdentity({ namespace: 'dev', name: 'app', uid: 'new' }));
  assert.equal(getSchedulingDetailError({ response: { status: 409 } }), 'recreated');
  assert.equal(getSchedulingDetailError({ response: { status: 404 } }), 'gone');
  assert.equal(getSchedulingDetailError(new Error()), 'loadFailed');
});

test('all scheduling translations have matching keys', () => {
  const paths = (object, prefix = '') => Object.entries(object).flatMap(([key, value]) => (
    typeof value === 'object' ? paths(value, `${prefix}${key}.`) : [`${prefix}${key}`]
  ));
  assert.deepEqual(paths(zh), paths(en));
});

test('GPU requests compose fixed tiles and mark unset values', () => {
  const r = (name, value, kind, unit = '') => ({ name, value, kind, unit });
  assert.deepEqual(getSchedulingRequestTiles({ resources: [
    r('nvidia.com/gpu', '2', 'count'), r('nvidia.com/gpucores', '50', 'core', '%'), r('nvidia.com/gpumem', '40960', 'memory', 'MiB'),
  ] }), [{ kind: 'count', value: '2' }, { kind: 'core', value: '50%' }, { kind: 'memory', value: '40 GiB' }]);
  assert.deepEqual(getSchedulingRequestTiles({ resources: [r('nvidia.com/gpu', '1', 'count'), r('nvidia.com/gpumem-percentage', '25', 'memory_percentage', '%')] }), [
    { kind: 'count', value: '1' }, { kind: 'core', value: null }, { kind: 'memory_percentage', value: '25%' },
  ]);
  assert.deepEqual(getSchedulingRequestTiles({ resources: [r('example.com/vgpu', '3', 'raw')] }), [
    { kind: 'raw', name: 'example.com/vgpu', value: '3' },
  ]);
  const ascend = { resources: [
    { ...r('huawei.com/Ascend910B3', '1', 'count'), vendor: 'Ascend' },
    { ...r('huawei.com/Ascend910B3-memory', '16384', 'memory', 'MiB'), vendor: 'Ascend' },
  ] };
  assert.deepEqual(getSchedulingRequestTiles(ascend), [
    { kind: 'count', value: '1', vendor: 'Ascend' }, { kind: 'core', value: null }, { kind: 'memory', value: '16 GiB', vendor: 'Ascend' },
  ]);
  assert.deepEqual(getWorkloadRequestTotals(ascend), { count: 1, cores: null, memoryMiB: 16384 });
});

test('scheduling constraints become readable groups and keep unparsed values', () => {
  const affinity = {
    nodeAffinity: { requiredDuringSchedulingIgnoredDuringExecution: { nodeSelectorTerms: [
      { matchExpressions: [{ key: 'zone', operator: 'In', values: ['a', 'b'] }] },
      { matchExpressions: [{ key: 'kubernetes.io/hostname', operator: 'NotIn', values: ['n1'] }] },
    ] } },
    podAntiAffinity: { preferredDuringSchedulingIgnoredDuringExecution: [
      { weight: 100, podAffinityTerm: { labelSelector: { matchLabels: { app: 'x' } }, topologyKey: 'kubernetes.io/hostname' } },
    ] },
  };
  const view = describeSchedulingConstraints([
    { name: 'nodeSelector.pool', value: 'inference' },
    { name: 'nvidia.com/use-gputype', value: 'A100, H100,' },
    { name: 'hami.io/gpu-scheduler-policy', value: 'binpack,spread' },
    { name: 'priorityClassName', value: 'research-high' },
    { name: 'tolerations', value: JSON.stringify([{ key: 'dedicated', operator: 'Equal', value: 'training', effect: 'NoSchedule' }, { operator: 'Exists' }]) },
    { name: 'affinity', value: JSON.stringify(affinity) },
    { name: 'topologySpreadConstraints', value: 'not json' },
    { name: 'example.com/custom', value: 'x' },
  ], ['example.com/hold']);
  assert.deepEqual(view.nodeSelector, ['pool=inference']);
  assert.deepEqual(view.device, [
    { key: 'useGpuType', name: 'nvidia.com/use-gputype', values: ['A100', 'H100'] },
    { key: 'gpuPolicy', name: 'hami.io/gpu-scheduler-policy', values: ['binpack,spread'] },
  ]);
  assert.equal(view.priorityClassName, 'research-high');
  assert.deepEqual(view.tolerations, ['dedicated=training:NoSchedule', 'op=Exists']);
  assert.equal(formatToleration({ key: 'nvidia.com/gpu', operator: 'Exists', effect: 'NoSchedule' }), 'nvidia.com/gpu:NoSchedule op=Exists');
  assert.equal(formatToleration({ key: 'node.kubernetes.io/not-ready', operator: 'Exists', effect: 'NoExecute', tolerationSeconds: 300 }), 'node.kubernetes.io/not-ready:NoExecute op=Exists for 300s');
  assert.deepEqual(view.nodeAffinity, [{ mode: 'required', terms: [['zone In a, b'], ['kubernetes.io/hostname NotIn n1']] }]);
  assert.deepEqual(view.podAffinity, [{ kind: 'podAntiAffinity', mode: 'preferred', terms: [
    { selector: ['app=x'], topologyKey: 'kubernetes.io/hostname', namespaces: [], weight: 100 },
  ] }]);
  assert.deepEqual(view.topology, []);
  assert.deepEqual(view.other, [{ name: 'example.com/custom', value: 'x' }]);
  assert.deepEqual(view.gates, ['example.com/hold']);
  assert.deepEqual(view.raw.map((item) => item.name), ['tolerations', 'affinity', 'topologySpreadConstraints']);
  assert.equal(view.raw[2].text, 'not json');
  assert.match(view.raw[1].text, /\n {2}"nodeAffinity"/);
});

test('JSON highlighting keeps every character as text', () => {
  const text = JSON.stringify({ a: 1, b: [true, '<b>x</b>', null] }, null, 2);
  const tokens = tokenizeJSON(text);
  assert.equal(tokens.map((token) => token.text).join(''), text);
  assert.deepEqual(tokens.filter((token) => token.type !== 'plain').map((token) => [token.type, token.text]), [
    ['key', '"a"'], ['number', '1'], ['key', '"b"'], ['literal', 'true'], ['string', '"<b>x</b>"'], ['literal', 'null'],
  ]);
});

test('record times read relative to the last refresh', () => {
  const at = '2026-09-12T08:00:00Z';
  const later = (offset) => Date.parse(at) + offset;
  for (const messages of [zh, en]) {
    language = messages;
    const ago = (duration) => messages.ago.replace('{duration}', duration);
    assert.equal(formatSchedulingDuration(at, later(90 * 60_000), t), messages.duration.hours.replace('{count}', '1'));
    assert.equal(formatSchedulingAgo(at, later(59_000), t), messages.justNow);
    assert.equal(formatSchedulingAgo(at, later(-5_000), t), messages.justNow, 'A slightly ahead server clock reads as just now');
    assert.equal(formatSchedulingAgo(at, later(3 * 60_000), t), ago(messages.duration.minutes.replace('{count}', '3')));
    assert.equal(formatSchedulingAgo(at, later(2 * 86_400_000), t), ago(messages.duration.days.replace('{count}', '2')));
    assert.equal(formatSchedulingAgo('not-a-time', later(0), t), '');
    assert.equal(formatSchedulingAgo(at, later(-5 * 60_000), t), '', 'A timestamp well ahead of the browser clock falls back to the exact time');
  }
  language = zh;
});

test('pending wait time uses one coarse unit and omits invalid or future timestamps', () => {
  const created = '2026-09-12T08:00:00Z';
  const at = (offset) => Date.parse(created) + offset;
  for (const messages of [zh, en]) {
    language = messages;
    const waited = (duration) => messages.waitedFor.replace('{duration}', duration);
    assert.equal(formatSchedulingWait(created, at(30_000), t), waited(messages.duration.lessThanMinute));
    assert.equal(formatSchedulingWait(created, at(25 * 60_000 + 59_000), t), waited(messages.duration.minutes.replace('{count}', '25')));
    assert.equal(formatSchedulingWait(created, at(3 * 3_600_000 + 59 * 60_000), t), waited(messages.duration.hours.replace('{count}', '3')));
    assert.equal(formatSchedulingWait(created, at(2 * 86_400_000), t), waited(messages.duration.days.replace('{count}', '2')));
    for (const [value, now] of [['', at(0)], ['not-a-time', at(0)], [created, at(-1)], [created, Number.NaN]]) {
      assert.equal(formatSchedulingWait(value, now, t), '');
    }
  }
  language = zh;
});
