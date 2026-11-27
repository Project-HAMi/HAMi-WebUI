import assert from 'node:assert/strict';
import test from 'node:test';

import { formatOptionalTelemetry } from './optional-telemetry-display.mjs';

test('optional telemetry distinguishes missing and non-finite values from zero', () => {
  for (const value of [undefined, null, '', Number.NaN, Infinity, -Infinity]) {
    assert.equal(formatOptionalTelemetry(value, 'W'), '--');
  }

  assert.equal(formatOptionalTelemetry(0, 'W'), '0 W');
  assert.equal(formatOptionalTelemetry(42.5, 'W'), '42.5 W');
  assert.equal(formatOptionalTelemetry(0, '℃'), '0 ℃');
  assert.equal(formatOptionalTelemetry(65, '℃'), '65 ℃');
});
