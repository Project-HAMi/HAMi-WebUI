import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import { buildDonutOptions, buildTimeSeriesOptions } from './chart-presets.mjs';
import { CHART_COLORS, categoricalColor } from './chart-colors.mjs';
import { formatTimeAxisTooltip } from './time-axis.mjs';

const plottedValues = (series) => series.data.map((point) => point.value ?? point);

const points = (...values) => values.map((value, i) => ({ timestamp: 1700000000 + i * 60, value }));

test('tooltips retain the full local date, seconds and UTC offset', () => {
  const originalTZ = process.env.TZ;
  process.env.TZ = 'Asia/Shanghai';
  try {
    assert.equal(formatTimeAxisTooltip(Date.parse('2026-10-07T04:00:45Z')), '2026-10-07 12:00:45 UTC+08:00');
    assert.equal(formatTimeAxisTooltip(Date.parse('2026-12-31T16:00:00Z')), '2027-01-01 00:00:00 UTC+08:00');
  } finally {
    if (originalTZ === undefined) delete process.env.TZ;
    else process.env.TZ = originalTZ;
  }
});

test('a repeated DST hour keeps distinct instants and identifies each point offset', () => {
  const originalTZ = process.env.TZ;
  process.env.TZ = 'America/New_York';
  try {
    const timestamps = ['2026-11-01T05:30:00Z', '2026-11-01T06:30:00Z'].map(Date.parse);
    const option = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: timestamps.map((timestamp, i) => ({ timestamp, value: i + 1 })) }] });
    assert.deepEqual(plottedValues(option.series[0]), [[timestamps[0], 1], [timestamps[1], 2]]);
    for (const [index, offset] of [[0, '-04:00'], [1, '-05:00']]) {
      const tooltip = option.tooltip.formatter([{ axisValue: timestamps[index], value: plottedValues(option.series[0])[index] }]);
      assert.ok(tooltip.includes(`2026-11-01 01:30:00 UTC${offset}`));
    }
  } finally {
    if (originalTZ === undefined) delete process.env.TZ;
    else process.env.TZ = originalTZ;
  }
});

test('invalid timestamps remain gaps without moving valid samples to a different instant', () => {
  assert.deepEqual(buildTimeSeriesOptions().series, []);
  const option = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: [{ timestamp: null, value: 2 }, { timestamp: 0, value: 0 }, { timestamp: Infinity, value: 3 }, { timestamp: 9e20, value: 4 }] }] });
  assert.deepEqual(plottedValues(option.series[0]), [[null, null], [0, 0], [null, null], [null, null]]);
  for (const value of [null, undefined, '', 'invalid:0', '9e20']) assert.equal(formatTimeAxisTooltip(value), null);
});

test('a time series draws the values as read and rounds only in the tooltip', () => {
  const option = buildTimeSeriesOptions({
    series: [{ name: 'Allocation', status: REQUEST_STATUS.READY, data: points(0.04, 0, 37.524) }],
  });

  assert.deepEqual(plottedValues(option.series[0]), points(0.04, 0, 37.524).map(({ timestamp, value }) => [timestamp, value]));
  const tooltip = option.tooltip.formatter([
    { axisValueLabel: '12:00', seriesName: 'Allocation', color: '#5B8FF9', value: 0.004 },
    { axisValueLabel: '12:00', seriesName: 'Idle', color: '#42C090', value: 0 },
    { axisValueLabel: '12:00', seriesName: 'Usage', color: '#42C090', value: 37.524 },
  ]);
  assert.match(tooltip, /&lt;0\.01 %/);
  assert.match(tooltip, />0\.00 %</);
  assert.match(tooltip, />37\.52 %</);
});

test('a time series draws unreadable samples as gaps, never as zero', () => {
  const option = buildTimeSeriesOptions({
    series: [{ name: 'Usage', data: points('NaN', Infinity, null, '', 0, 0.04) }],
  });
  assert.deepEqual(plottedValues(option.series[0]), points(null, null, null, null, 0, 0.04).map(({ timestamp, value }) => [timestamp, value]));
  const tooltip = option.tooltip.formatter([
    { axisValueLabel: '12:00', seriesName: 'Usage', color: '#42C090', value: null },
    { axisValueLabel: '12:00', seriesName: 'Usage', color: '#42C090', value: 'NaN' },
  ]);
  assert.equal(tooltip.match(/>-</g).length, 2);
});

test('a time series tooltip prints cluster names as text', () => {
  const option = buildTimeSeriesOptions({ series: [{ name: 'a', data: points(1) }] });
  const tooltip = option.tooltip.formatter([
    { axisValueLabel: '12:00', seriesName: 'node<img src=x onerror=alert(1)>', color: '#5B8FF9', value: 1 },
  ]);
  assert.doesNotMatch(tooltip, /<img/);
  assert.match(tooltip, /node&lt;img/);
});

test('a time series shows a legend only for more than one line and keeps gaps', () => {
  const single = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: points(1, null, 3) }] });
  assert.equal(single.legend.show, false);
  assert.deepEqual(plottedValues(single.series[0]), points(1, null, 3).map(({ timestamp, value }) => [timestamp, value]));
  assert.equal(single.series[0].connectNulls, false);

  const pair = buildTimeSeriesOptions({
    series: [{ name: 'Allocation', data: points(1) }, { name: 'Usage', data: points(2) }],
  });
  assert.notEqual(pair.legend.show, false);
  assert.ok(pair.grid.bottom > single.grid.bottom);
});

test('a failed line does not remove the ready line timestamp coordinates', () => {
  const option = buildTimeSeriesOptions({
    series: [
      { name: 'Allocation', status: REQUEST_STATUS.ERROR, data: [] },
      { name: 'Usage', status: REQUEST_STATUS.READY, data: points(1, 2) },
    ],
  });
  assert.deepEqual(plottedValues(option.series[0]), []);
  assert.deepEqual(plottedValues(option.series[1]), points(1, 2).map(({ timestamp, value }) => [timestamp, value]));
});

test('a donut tooltip prints cluster names as text', () => {
  const option = buildDonutOptions({ data: [{ name: 'A100<b>', value: 2 }], unit: 'cards' });
  assert.equal(option.tooltip.formatter({ name: 'A100<b>', value: 2 }), 'A100&lt;b&gt;: 2 cards');
});

test('a legend past the palette repeats the colours the donut draws', () => {
  const palette = buildDonutOptions().color;
  assert.deepEqual(palette, CHART_COLORS.categorical);
  const colors = Array.from({ length: palette.length + 2 }, (_, i) => categoricalColor(i));
  assert.ok(colors.every(Boolean));
  assert.equal(colors[palette.length], palette[0]);
  assert.equal(colors[palette.length + 1], palette[1]);
});
