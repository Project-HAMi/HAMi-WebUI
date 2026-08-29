import assert from 'node:assert/strict';
import test from 'node:test';

import { createComputeUsageGaugeConfig } from './metric-config.mjs';

test('compute utilization is treated as an already-normalized percentage', () => {
  const config = createComputeUsageGaugeConfig();

  assert.equal(config.total, 100);
  assert.equal(config.totalQuery, undefined);
  assert.match(config.query, /hami_core_util/);
  assert.equal((41 / config.total) * 100, 41);
});
