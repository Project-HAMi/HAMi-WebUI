import assert from 'node:assert/strict';
import test from 'node:test';
import { fitPopupToOptions } from './select-popup.mjs';

const popupOf = (width) => ({ getBoundingClientRect: () => ({ width }) });
const fitted = (minWidth) => ({ width: 'max-content', minWidth, maxWidth: '1000px' });

test('popups follow their options from the rounded-up first width, the trigger width and up to 1000px', () => {
  assert.deepEqual(fitPopupToOptions({ offsetWidth: 193 }, popupOf(219.08)), fitted('220px'));
  assert.deepEqual(fitPopupToOptions({ offsetWidth: 193 }, popupOf(219)), fitted('219px'));
  assert.deepEqual(fitPopupToOptions({ offsetWidth: 240 }, popupOf(180.5)), fitted('240px'));
  assert.deepEqual(fitPopupToOptions({ offsetWidth: 240 }, popupOf(1400.2)), fitted('1000px'));
});
