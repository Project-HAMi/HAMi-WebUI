import assert from 'node:assert/strict';
import test from 'node:test';

import {
  resolveLanguage,
  toDocumentLanguage,
} from '../src/locales/language.mjs';

test('a supported saved preference overrides the browser language', () => {
  assert.equal(resolveLanguage('en', 'zh-CN'), 'en');
  assert.equal(resolveLanguage('zh', 'en-US'), 'zh');
});

test('Chinese browser locales resolve to Chinese', () => {
  assert.equal(resolveLanguage(undefined, 'zh'), 'zh');
  assert.equal(resolveLanguage(undefined, 'zh-CN'), 'zh');
  assert.equal(resolveLanguage(undefined, 'zh_TW'), 'zh');
});

test('other browser locales use the English fallback', () => {
  assert.equal(resolveLanguage(undefined, 'en-US'), 'en');
  assert.equal(resolveLanguage(undefined, 'ja-JP'), 'en');
  assert.equal(resolveLanguage(undefined, undefined), 'en');
});

test('an invalid saved preference does not become an application locale', () => {
  assert.equal(resolveLanguage('fr', 'zh-CN'), 'zh');
  assert.equal(resolveLanguage('zh-CN', 'en-US'), 'en');
});

test('application locales map to document language tags', () => {
  assert.equal(toDocumentLanguage('en'), 'en');
  assert.equal(toDocumentLanguage('zh'), 'zh-CN');
  assert.equal(toDocumentLanguage('fr'), 'en');
});
