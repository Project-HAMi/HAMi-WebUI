import assert from 'node:assert/strict';
import test from 'node:test';

import en from './en.js';
import zh from './zh.js';

test('workload allocation column names the resource configuration', () => {
  assert.equal(zh.task.card, '算力配置');
  assert.equal(en.task.card, 'Accelerator Allocation');
});

test('GPU counts use quantity language consistently', () => {
  assert.equal(zh.dashboard.gpuCardCount, 'GPU 数量');
  assert.equal(zh.task.gpuCardCount, 'GPU 数量');
});

test('interface copy calls GPU jobs workloads, not tasks', () => {
  const strings = (value, path = '') => (typeof value === 'string' ? [[path, value]]
    : Object.entries(value).flatMap(([key, item]) => strings(item, path ? `${path}.${key}` : key)));
  assert.deepEqual(strings(zh).filter(([, text]) => text.includes('任务')).map(([path]) => path), []);
  assert.deepEqual(strings(en).filter(([, text]) => /\btasks?\b/i.test(text)).map(([path]) => path), []);
  assert.equal(zh.task.title, zh.routes.tasks);
  assert.equal(en.task.title, en.routes.tasks);
});
