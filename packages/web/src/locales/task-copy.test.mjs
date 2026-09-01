import assert from 'node:assert/strict';
import test from 'node:test';

import en from './en.js';
import zh from './zh.js';

test('workload allocation column names the resource configuration', () => {
  assert.equal(zh.task.card, '算力配置');
  assert.equal(en.task.card, 'Accelerator Allocation');
});
