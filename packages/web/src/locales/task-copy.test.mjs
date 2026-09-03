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
