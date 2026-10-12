import assert from 'node:assert/strict';
import test from 'node:test';
import { readdirSync } from 'node:fs';

import en from '../../../src/locales/en.js';
import zh from '../../../src/locales/zh.js';
import { getSplitIcon, getSplitModeKey, SPLIT_MODES } from './split-mode.mjs';

const lookup = (messages, key) => key.split('.').reduce((value, part) => value?.[part], messages);

test('registered split modes have copy and unknown ones stay blank', () => {
  assert.equal(getSplitModeKey('mig'), 'card.splitMode.mig');
  assert.equal(lookup(zh, getSplitModeKey('hami-core')), 'HAMi-core 模式');
  assert.equal(lookup(zh, getSplitModeKey('template')), '模板切分');
  for (const mode of SPLIT_MODES) {
    assert.equal(typeof lookup(en, getSplitModeKey(mode)), 'string', mode);
  }
  for (const mode of ['', undefined, 'something-new']) {
    assert.equal(getSplitModeKey(mode), '');
  }
});

test('partitioned splits share one icon and HAMi-core another', () => {
  const icons = readdirSync(new URL('../../../src/icons/svg', import.meta.url)).map((file) => file.replace(/\.svg$/, ''));
  assert.equal(getSplitIcon('mig'), getSplitIcon('template'));
  assert.equal(getSplitIcon('hami-core'), getSplitIcon('soft'));
  assert.notEqual(getSplitIcon('mig'), getSplitIcon('hami-core'));
  for (const key of ['mig', 'template', 'hami-core', 'soft', 'whole']) {
    assert.ok(icons.includes(getSplitIcon(key)), `${key} -> ${getSplitIcon(key)}`);
  }
  assert.equal(getSplitIcon('unknown'), '');
  assert.equal(getSplitIcon(undefined), '');
});
