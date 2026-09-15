import assert from 'node:assert/strict';
import test from 'node:test';

import en from '../../../src/locales/en.js';
import zh from '../../../src/locales/zh.js';
import { deviceWording, sharedVendor } from './device-copy.mjs';

test('Ascend devices are called NPUs and other providers keep GPU wording', () => {
  assert.equal(deviceWording(zh.card.model, 'Ascend'), 'NPU 型号');
  assert.equal(deviceWording(zh.task.relatedGpu, 'Ascend'), '关联 NPU');
  assert.equal(deviceWording(zh.scheduling.resourceKind.count, 'Ascend'), 'NPU 数量');
  assert.equal(deviceWording(zh.scheduling.reason.memory, 'Ascend'), 'NPU 可分配显存不足');
  assert.equal(deviceWording(en.card.model, 'Ascend'), 'NPU Model');
  assert.equal(deviceWording('Check GPUs and vGPU slots', 'Ascend'), 'Check NPUs and vGPU slots');
  assert.equal(deviceWording(zh.card.model, 'NVIDIA'), zh.card.model);
  assert.equal(deviceWording(zh.card.model, ''), zh.card.model);
});

test('a vendor is shared only when every item reports the same one', () => {
  assert.equal(sharedVendor(['Ascend', 'Ascend']), 'Ascend');
  assert.equal(sharedVendor(['Ascend', 'NVIDIA']), '');
  assert.equal(sharedVendor(['Ascend', undefined]), '');
  assert.equal(sharedVendor([]), '');
});
