import assert from 'node:assert/strict';
import test from 'node:test';

import en from './en.js';
import zh from './zh.js';

test('the unified workload column describes both requested and allocated resources', () => {
  assert.equal(zh.task.resourceConfiguration, '算力配置');
  assert.equal(en.task.resourceConfiguration, 'Accelerator Configuration');
});

test('GPU counts use quantity language consistently', () => {
  assert.equal(zh.dashboard.gpuCardCount, 'GPU 数量');
  assert.equal(zh.task.gpuCardCount, 'GPU 数量');
});
