import assert from 'node:assert/strict';
import test from 'node:test';

import en from '../../../../../src/locales/en.js';
import zh from '../../../../../src/locales/zh.js';
import {
  affectsAscendDevices,
  buildAllocationOptions,
  DEVICE_CONFIG_STATES,
  findAscendModel,
  getDeviceConfigStateKey,
  shouldWarnAboutDeviceConfig,
} from './device-config-display.mjs';

const lookup = (messages, key) => key.split('.').reduce((value, part) => value?.[part], messages);

// Shape of GET /v1/device-config: protojson encodes int64 as strings.
const config = {
  state: 'loaded',
  ascendModels: [
    {
      commonWord: 'Ascend910B3',
      chipName: '910B3',
      memoryAllocatable: '65536',
      aiCore: 20,
      aiCpu: 7,
      templates: [
        { name: 'vir05_1c_16g', memory: '16384', aiCore: 5, aiCpu: 1, computeShare: 25 },
        { name: 'vir10_3c_32g', memory: '32768', aiCore: 10, aiCpu: 3, computeShare: 50 },
      ],
    },
    { commonWord: 'Custom910', memoryAllocatable: '1000', aiCore: 0, aiCpu: 0, templates: [{ name: 't1', memory: '500', aiCore: 0, aiCpu: 0 }] },
  ],
};

test('Ascend models are found by commonWord with numeric fields', () => {
  assert.deepEqual(findAscendModel(config, 'Ascend910B3'), {
    commonWord: 'Ascend910B3',
    chipName: '910B3',
    aiCore: 20,
    aiCpu: 7,
    memoryAllocatableMiB: 65536,
    superPod: false,
    templates: [
      { name: 'vir05_1c_16g', memoryMiB: 16384, aiCore: 5, aiCpu: 1, computeShare: 25 },
      { name: 'vir10_3c_32g', memoryMiB: 32768, aiCore: 10, aiCpu: 3, computeShare: 50 },
    ],
  });
  const custom = findAscendModel(config, 'Custom910');
  assert.equal(custom.aiCore, undefined);
  assert.equal(custom.templates[0].computeShare, undefined);
});

test('models are not guessed from partial or unread configuration', () => {
  assert.equal(findAscendModel(config, 'ascend910b3'), undefined);
  assert.equal(findAscendModel(config, 'Ascend910B4'), undefined);
  assert.equal(findAscendModel(config, ''), undefined);
  assert.equal(findAscendModel({ ...config, state: 'forbidden' }, 'Ascend910B3'), undefined);
  assert.equal(findAscendModel(undefined, 'Ascend910B3'), undefined);
});

test('unread configuration states have copy in both languages', () => {
  assert.equal(getDeviceConfigStateKey(config), '');
  assert.equal(getDeviceConfigStateKey({ state: 'forbidden' }), 'card.deviceConfig.state.forbidden');
  assert.equal(getDeviceConfigStateKey({ state: 'something-new' }), 'card.deviceConfig.state.error');
  assert.equal(getDeviceConfigStateKey(undefined), 'card.deviceConfig.state.error');
  for (const state of DEVICE_CONFIG_STATES) {
    assert.equal(typeof lookup(zh, `card.deviceConfig.state.${state}`), 'string', `zh ${state}`);
    assert.equal(typeof lookup(en, `card.deviceConfig.state.${state}`), 'string', `en ${state}`);
  }
});

test('allocation options show each template as a share of one card, then the whole card', () => {
  const options = buildAllocationOptions(findAscendModel(config, 'Ascend910B3'));
  assert.deepEqual(options.map(({ name, whole, computeShare }) => [name, whole, computeShare]), [
    ['vir05_1c_16g', false, 25], ['vir10_3c_32g', false, 50], ['', true, 100],
  ]);
  const [small, , whole] = options;
  assert.deepEqual(small.aiCore, { used: 5, total: 20, percent: 25 });
  assert.deepEqual(small.aiCpu, { used: 1, total: 7, percent: 14 });
  assert.deepEqual(small.memory, { used: 16384, total: 65536, percent: 25 });
  assert.deepEqual([whole.aiCore.percent, whole.memory.percent], [100, 100]);

  const custom = buildAllocationOptions(findAscendModel(config, 'Custom910'));
  assert.deepEqual([custom[0].aiCore.percent, custom[0].computeShare], [undefined, undefined]);
  assert.deepEqual(buildAllocationOptions(undefined), []);
});

test('an unreadable configuration warns only where Ascend devices exist', () => {
  const ascend = ['NVIDIA-A100-SXM4-40GB', 'Ascend910B3'];
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'forbidden' }, ascend), true);
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'missing' }, ascend), true);
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'invalid' }, ascend), true);
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'disabled' }, ascend), true);
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'loaded' }, ascend), false);
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'loading' }, ascend), false);
  assert.equal(shouldWarnAboutDeviceConfig({ state: 'forbidden' }, ['NVIDIA-A100-SXM4-40GB']), false);
  assert.equal(shouldWarnAboutDeviceConfig(undefined, ascend), false);
  assert.equal(affectsAscendDevices(), false);
});
