import assert from 'node:assert/strict';
import test from 'node:test';

import { buildTimeSeriesOptions } from '../../projects/vgpu/metrics/chart-presets.mjs';
import { timeParse } from './index.js';
import { formatQueryTimestamp } from './query-time.mjs';

test('query timestamps identify the same instant across explicit time zones', () => {
  for (const value of [
    '2026-10-07T04:00:00.999Z',
    '2026-10-07T12:00:00.999+08:00',
    '2026-10-07T00:00:00.999-04:00',
  ]) {
    assert.equal(formatQueryTimestamp(new Date(value)), '2026-10-07T04:00:00Z');
  }
});

test('query timestamps truncate milliseconds without rounding or mutating the input', () => {
  const value = new Date('2026-10-07T23:59:59.999Z');
  assert.equal(formatQueryTimestamp(value), '2026-10-07T23:59:59Z');
  assert.equal(value.toISOString(), '2026-10-07T23:59:59.999Z');
  assert.equal(formatQueryTimestamp(new Date('2026-10-08T00:00:00.001Z')), '2026-10-08T00:00:00Z');
});

test('explicit offsets preserve distinct instants in the repeated DST hour', () => {
  assert.equal(formatQueryTimestamp(new Date('2026-11-01T01:30:00-04:00')), '2026-11-01T05:30:00Z');
  assert.equal(formatQueryTimestamp(new Date('2026-11-01T01:30:00-05:00')), '2026-11-01T06:30:00Z');
});

test('UTC query serialization leaves input, axis, and tooltip formatting in local time', () => {
  const originalTZ = process.env.TZ;
  const value = new Date('2026-10-07T12:00:00.987Z');
  try {
    for (const [timezone, localTime] of [
      ['UTC', '12:00'],
      ['Asia/Shanghai', '20:00'],
      ['America/New_York', '08:00'],
    ]) {
      process.env.TZ = timezone;
      const label = `2026-10-07 ${localTime}:00`;
      const options = buildTimeSeriesOptions({
        series: [{ name: 'Usage', data: [{ timestamp: value.getTime(), value: 42 }] }],
      });
      assert.equal(formatQueryTimestamp(value), '2026-10-07T12:00:00Z');
      assert.equal(timeParse(value), label);
      assert.deepEqual(options.series[0].data.map((point) => point.value ?? point), [[value.getTime(), 42]]);
      assert.ok(options.tooltip.formatter([{ axisValue: String(value.getTime()), seriesName: 'Usage', value: 42 }]).includes(label));
    }
  } finally {
    if (originalTZ === undefined) delete process.env.TZ;
    else process.env.TZ = originalTZ;
  }
});
