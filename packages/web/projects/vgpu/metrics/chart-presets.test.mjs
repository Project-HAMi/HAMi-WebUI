import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import { buildDonutOptions, buildTimeSeriesOptions } from './chart-presets.mjs';
import { CHART_COLORS, categoricalColor } from './chart-colors.mjs';

const points = (...values) => values.map((value, i) => ({ timestamp: 1700000000 + i * 60, value }));

test('a time series draws the values as read and rounds only in the tooltip', () => {
  const option = buildTimeSeriesOptions({
    series: [{ name: 'Allocation', status: REQUEST_STATUS.READY, data: points(0.04, 0, 37.524) }],
  });

  assert.deepEqual(option.series[0].data, [0.04, 0, 37.524]);
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
  assert.deepEqual(option.series[0].data, [null, null, null, null, 0, 0.04]);
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
  assert.deepEqual(single.series[0].data, [1, null, 3]);
  assert.equal(single.series[0].connectNulls, false);

  const pair = buildTimeSeriesOptions({
    series: [{ name: 'Allocation', data: points(1) }, { name: 'Usage', data: points(2) }],
  });
  assert.notEqual(pair.legend.show, false);
  assert.ok(pair.grid.bottom > single.grid.bottom);
});

test('a time series takes its axis from a ready line when another failed', () => {
  const option = buildTimeSeriesOptions({
    series: [
      { name: 'Allocation', status: REQUEST_STATUS.ERROR, data: [] },
      { name: 'Usage', status: REQUEST_STATUS.READY, data: points(1, 2) },
    ],
  });
  assert.equal(option.xAxis.data.length, 2);
  assert.deepEqual(option.series[1].data, [1, 2]);
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
