import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildRangeLineSeries,
  formatRangeTooltipValue,
  normalizeRangeValues,
  normalizeRangeVectorResponse,
  toFiniteRangeValue,
} from './range-vector-state.mjs';

test('range normalization preserves slots while separating zero from missing data', () => {
  const values = normalizeRangeValues([
    { timestamp: '1000', value: 0 },
    { timestamp: '2000', value: 0, missing: true },
    { timestamp: '3000', value: 'NaN' },
    { timestamp: '4000', value: 'Infinity' },
    { timestamp: '5000', value: '-Infinity' },
    { timestamp: '6000', value: '12.5' },
    ['7000', 0],
  ]);

  assert.equal(values.length, 7);
  assert.deepEqual(
    values.map(({ timestamp, value }) => ({ timestamp, value })),
    [
      { timestamp: 1000, value: 0 },
      { timestamp: 2000, value: null },
      { timestamp: 3000, value: null },
      { timestamp: 4000, value: null },
      { timestamp: 5000, value: null },
      { timestamp: 6000, value: 12.5 },
      { timestamp: 7000, value: 0 },
    ],
  );
  assert.equal(values[1].missing, true);
});

test('non-numeric range values do not become synthetic zeroes', () => {
  assert.equal(toFiniteRangeValue(null), null);
  assert.equal(toFiniteRangeValue(undefined), null);
  assert.equal(toFiniteRangeValue(''), null);
  assert.equal(toFiniteRangeValue(false), null);
  assert.equal(toFiniteRangeValue('0'), 0);
  assert.equal(toFiniteRangeValue(0), 0);
});

test('range response normalization is pure and retains every stream slot', () => {
  const response = {
    code: 0,
    data: [
      {
        metric: { device_uuid: 'GPU-1' },
        values: [
          { timestamp: '1000', value: 0 },
          { timestamp: '2000', missing: true },
        ],
      },
    ],
  };

  const normalized = normalizeRangeVectorResponse(response);

  assert.notEqual(normalized, response);
  assert.notEqual(normalized.data, response.data);
  assert.equal(normalized.data[0].values.length, 2);
  assert.deepEqual(
    normalized.data[0].values.map(({ timestamp, value }) => ({
      timestamp,
      value,
    })),
    [
      { timestamp: 1000, value: 0 },
      { timestamp: 2000, value: null },
    ],
  );
  assert.equal(response.data[0].values[1].value, undefined);
});

test('live range chart series retain gaps and never reconnect missing slots', () => {
  const points = [
    { timestamp: 1000, value: 0 },
    { timestamp: 2000, missing: true },
    { timestamp: 3000, value: 2048 },
  ];
  const cases = [
    { name: 'overview', presentation: undefined, expected: [0, null, 2048] },
    { name: 'node detail', presentation: undefined, expected: [0, null, 2048] },
    {
      name: 'card common line',
      presentation: { digits: 1 },
      expected: [0, null, 2048],
    },
    {
      name: 'task common line',
      presentation: { digits: 1 },
      expected: [0, null, 2048],
    },
  ];

  for (const { name, presentation, expected } of cases) {
    const series = buildRangeLineSeries({ name, data: points }, presentation);
    assert.deepEqual(series.data, expected, name);
    assert.equal(series.connectNulls, false, name);
  }
});

test('range tooltips render missing values as a dash without hiding real zero', () => {
  assert.equal(formatRangeTooltipValue(null, { digits: 1, unit: '%' }), '-');
  assert.equal(formatRangeTooltipValue('NaN', { digits: 1, unit: '%' }), '-');
  assert.equal(formatRangeTooltipValue(0, { digits: 1, unit: '%' }), '0.0%');
});
