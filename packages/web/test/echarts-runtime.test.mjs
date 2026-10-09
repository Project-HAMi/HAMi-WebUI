import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';

import { SVGRenderer } from 'echarts/renderers';

import { init } from '../src/plugins/echarts.mjs';
import { use } from 'echarts/core';
import { buildTimeSeriesOptions } from '../projects/vgpu/metrics/chart-presets.mjs';
import { layoutTimeSeriesOptions } from '../projects/vgpu/metrics/time-axis.mjs';

use([SVGRenderer]);

function renderedTexts(chart) {
  return chart.getZr().storage.getDisplayList()
    .filter((element) => element.type === 'tspan' && element.style.text && !element.ignore)
    .map((element) => {
      const rect = element.getBoundingRect().clone();
      if (element.transform) rect.applyTransform(element.transform);
      return { text: element.style.text, rect };
    });
}

const isTimeLabel = (text) => /^(?:\d{2}:\d{2}|(?:\d{4}-)?\d{2}-\d{2}|UTC[+-]|\d{4}$)/.test(text);

test('time scales position unequal sampling intervals proportionally and retain missing values', () => {
  const chart = init(null, null, { renderer: 'svg', ssr: true, width: 800, height: 250 });
  const start = Date.parse('2026-10-07T04:00:37Z');
  const data = [
    { timestamp: start, value: 10 },
    { timestamp: start + 60_000, value: null },
    { timestamp: start + 180_000, value: 30 },
    { timestamp: start + 600_000, value: 40 },
  ];
  try {
    chart.setOption(buildTimeSeriesOptions({ series: [{ name: 'Usage', data }] }));
    const x = data.map(({ timestamp }) => chart.convertToPixel({ xAxisIndex: 0 }, timestamp));
    assert.ok(Math.abs((x[2] - x[0]) / (x[1] - x[0]) - 3) < 0.001);
    assert.ok(Math.abs((x[3] - x[0]) / (x[1] - x[0]) - 10) < 0.001);
    const plotted = chart.getModel().getSeriesByIndex(0).getData();
    assert.ok(Number.isNaN(plotted.get(plotted.mapDimension('y'), 1)));
    assert.equal(plotted.get(plotted.mapDimension('y'), 2), 30);
  } finally {
    chart.dispose();
  }
});

test('continuous samples stay unmarked while isolated zero reports and zoomed single samples remain visible', () => {
  const chart = init(null, null, { renderer: 'svg', ssr: true, width: 800, height: 250 });
  const start = Date.parse('2026-10-09T04:00:00Z');
  const values = [10, 10, null, 0, null, 30, 30];
  const points = values.map((value, index) => ({ timestamp: start + index * 60_000, value }));
  const options = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: points }] });
  const symbolAt = (index) => chart.getModel().getSeriesByIndex(0).getData().getItemGraphicEl(index)?.childrenRef()[0];
  try {
    chart.setOption(options);
    for (const index of [0, 1, 5, 6]) assert.equal(symbolAt(index).style.opacity, 0, `Continuous sample ${index} shows a permanent marker`);
    assert.equal(symbolAt(3).style.opacity, 1, 'An isolated zero-valued report is invisible');
    assert.equal(symbolAt(2), undefined, 'Missing data must not create a marker');
    assert.equal(symbolAt(4), undefined, 'Missing data must not create a marker');
    const line = chart.getZr().storage.getDisplayList().find((element) => element.type === 'ec-polyline');
    assert.ok(line && line.style.opacity !== 0, 'Hiding markers hid the line itself');
    const originalData = chart.getModel().getSeriesByIndex(0).getData();
    assert.equal(originalData.get(originalData.mapDimension('y'), 3), 0, 'The isolated report must retain its true zero value');

    const visibleExtent = [points[1].timestamp - 5_000, points[1].timestamp + 5_000];
    chart.dispatchAction({ type: 'dataZoom', startValue: visibleExtent[0], endValue: visibleExtent[1] });
    chart.setOption(layoutTimeSeriesOptions(options, 800, visibleExtent));
    assert.equal(chart.getModel().getSeriesByIndex(0).getData().count(), 1);
    assert.equal(symbolAt(0).style.opacity, 1, 'Zooming to a single sample leaves an empty plot');

    chart.dispatchAction({ type: 'dataZoom', start: 0, end: 100 });
    chart.setOption(layoutTimeSeriesOptions(options, 800));
    assert.equal(symbolAt(1).style.opacity, 0, 'Zooming back out leaves a permanent marker on the line');
    assert.equal(symbolAt(3).style.opacity, 1, 'Zooming back out loses the isolated zero report');
  } finally {
    chart.dispose();
  }
});

test('a sparse month keeps a visible marker for subpixel reports and restores lines when zoomed', () => {
  const start = Date.parse('2026-09-09T14:00:00Z');
  const points = Array.from({ length: 721 }, (_, index) => ({
    timestamp: start + index * 3_600_000,
    value: index === 650 ? 0 : (index >= 660 && index <= 672) || index >= 719 ? 11 : null,
  }));
  const options = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: points }] });
  for (const width of [320, 590]) {
    const chart = init(null, null, { renderer: 'svg', ssr: true, width, height: 250 });
    try {
      chart.setOption(layoutTimeSeriesOptions(options, width));
      const data = chart.getModel().getSeriesByIndex(0).getData();
      const symbolAt = (index) => data.getItemGraphicEl(index)?.childrenRef()[0];
      assert.equal(symbolAt(650).style.opacity, 1, `${width}px: the isolated zero report vanished`);
      assert.equal(symbolAt(720).style.opacity, 1, `${width}px: the subpixel tail needs a visible report marker`);
      assert.equal(symbolAt(719).style.opacity, 0, 'A short fragment must not become a cluster of overlapping markers');
      assert.equal(data.getItemVisual(720, 'symbol'), 'circle');
      assert.equal(data.getItemVisual(720, 'symbolSize'), 4);
      assert.equal(symbolAt(720).style.fill, symbolAt(720).style.stroke, 'The report marker should be solid');
      for (let index = 660; index <= 672; index += 1) assert.equal(symbolAt(index).style.opacity, 0, 'The discernible 12-hour line must stay free of markers');
      assert.equal(symbolAt(718), undefined, 'The missing interval must remain empty');
      assert.equal(data.get(data.mapDimension('x'), 720), points[720].timestamp, 'A visibility marker must not move the report in time');
      assert.equal(data.get(data.mapDimension('y'), 650), 0, 'A visibility marker must not alter a zero-valued report');

      const zoom = [points[718].timestamp, points[720].timestamp];
      chart.dispatchAction({ type: 'dataZoom', startValue: zoom[0], endValue: zoom[1] });
      chart.setOption(layoutTimeSeriesOptions(options, width, zoom));
      const zoomed = chart.getModel().getSeriesByIndex(0).getData();
      assert.equal(zoomed.count(), 3);
      assert.equal(zoomed.getItemGraphicEl(0), undefined);
      for (const index of [1, 2]) assert.equal(zoomed.getItemGraphicEl(index).childrenRef()[0].style.opacity, 0, 'A clearly visible zoomed line retained a permanent marker');
    } finally {
      chart.dispose();
    }
  }
});

test('rendered time ticks use natural minutes and days rather than sample offsets', () => {
  const originalTZ = process.env.TZ;
  process.env.TZ = 'Asia/Shanghai';
  try {
    for (const hours of [1, 168, 720]) {
      const end = Date.parse('2026-10-09T13:50:37Z');
      const start = end - hours * 3_600_000;
      const chart = init(null, null, { renderer: 'svg', ssr: true, width: 700, height: 250 });
      try {
        chart.setOption(buildTimeSeriesOptions({ series: [{ name: 'Usage', data: Array.from({ length: 61 }, (_, index) => ({ timestamp: start + (end - start) * index / 60, value: index === 20 ? null : index })) }] }));
        const axis = chart.getModel().getComponent('xAxis').axis;
        const visible = axis.getViewLabels().filter(({ tick }) => tick.value > start && tick.value < end);
        assert.ok(visible.length >= 2, `${hours}h: useful time labels are missing`);
        for (const { tick, formattedLabel } of visible) {
          const date = new Date(tick.value);
          assert.equal(date.getSeconds(), 0, `${hours}h: tick retained sample seconds`);
          assert.equal(date.getMilliseconds(), 0);
          if (hours === 1) assert.match(formattedLabel, /^\d{2}:\d{2}$/);
          else {
            assert.equal(date.getHours(), 0, `${hours}h: tick is not a day boundary`);
            assert.equal(date.getMinutes(), 0);
            assert.match(formattedLabel, /^(?:\d{4}-)?\d{2}-\d{2}$/);
          }
        }
        const times = visible.map(({ tick }) => tick.value);
        const intervals = times.slice(1).map((time, index) => time - times[index]);
        assert.equal(new Set(intervals).size, 1, `${hours}h: irregular adjacent time ticks`);
      } finally {
        chart.dispose();
      }
    }
  } finally {
    if (originalTZ === undefined) delete process.env.TZ;
    else process.env.TZ = originalTZ;
  }
});

test('time labels fit narrow canvases and retain year and repeated-hour context', () => {
  const originalTZ = process.env.TZ;
  process.env.TZ = 'America/New_York';
  try {
    for (const [start, step] of [['2026-11-01T04:00:37Z', 180_000], ['2026-11-01T05:00:37Z', 120_000], ['2026-12-15T00:00:37Z', 43_200_000], ['2026-12-28T00:00:37Z', 10_080_000], ['2026-10-09T03:30:37Z', 60_000]]) {
      for (const width of [160, 208, 224, 300, 320, 460, 600]) {
        const chart = init(null, null, { renderer: 'svg', ssr: true, width, height: 250 });
        try {
          const options = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: Array.from({ length: 61 }, (_, index) => ({ timestamp: Date.parse(start) + index * step, value: index % 20 })) }] });
          chart.setOption(layoutTimeSeriesOptions(options, width));
          const labels = renderedTexts(chart).filter(({ text }) => isTimeLabel(text));
          assert.ok(labels.length >= 1, `${width}px: no readable time labels`);
          for (const { text, rect } of labels) {
            assert.ok(rect.x >= -0.5 && rect.x + rect.width <= width + 0.5, `${width}px: ${text} is clipped`);
          }
          for (let index = 0; index < labels.length; index += 1) {
            for (const previous of labels.slice(0, index)) {
              assert.ok(!labels[index].rect.intersect(previous.rect), `${width}px: ${labels[index].text} overlaps ${previous.text}`);
            }
          }
          if (width === 600) {
            const text = labels.map((label) => label.text).join(' ');
            if (start.includes('2026-12-')) assert.match(text, /2027/);
            else if (start.includes('11-01')) {
              assert.match(text, /UTC-04:00/);
              assert.match(text, /UTC-05:00/);
            } else assert.match(text, /10-09/);
          }
        } finally {
          chart.dispose();
        }
      }
    }
  } finally {
    if (originalTZ === undefined) delete process.env.TZ;
    else process.env.TZ = originalTZ;
  }
});

test('a one-second query and the strongest allowed zoom retain a readable time axis', () => {
  const start = Date.parse('2026-10-09T04:00:00Z');
  for (const duration of [1_000, 3_600_000]) {
    const chart = init(null, null, { renderer: 'svg', ssr: true, width: 320, height: 250 });
    try {
      const options = buildTimeSeriesOptions({ series: [{ name: 'Usage', data: [
        { timestamp: start, value: 10 },
        { timestamp: start + duration, value: 20 },
      ] }] });
      chart.setOption(layoutTimeSeriesOptions(options, 320));
      assert.ok(renderedTexts(chart).some(({ text }) => isTimeLabel(text)), 'A short original query has no time label');
      chart.dispatchAction({ type: 'dataZoom', start: 50, end: 50 });
      const zoom = chart.getOption().dataZoom[0];
      chart.setOption(layoutTimeSeriesOptions(options, 320, [zoom.startValue, zoom.endValue]));
      const labels = renderedTexts(chart).filter(({ text }) => isTimeLabel(text));
      assert.ok(labels.length >= 1, 'The strongest zoom has no time label');
      for (const { text, rect } of labels) {
        assert.ok(rect.x >= -0.5 && rect.x + rect.width <= 320.5, `Zoomed ${text} is clipped`);
      }
    } finally {
      chart.dispose();
    }
  }
});

const sourceRoots = ['src', 'projects'];
const sourceExtensions = new Set(['.js', '.mjs', '.vue']);

async function collectSourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map((entry) => {
      const entryPath = path.join(directory, entry.name);
      return entry.isDirectory() ? collectSourceFiles(entryPath) : [entryPath];
    }),
  );
  return files.flat();
}

test('production code never imports the complete ECharts bundle', async () => {
  const files = (
    await Promise.all(sourceRoots.map((root) => collectSourceFiles(root)))
  )
    .flat()
    .filter((file) => sourceExtensions.has(path.extname(file)));
  const fullImport = /(?:\bfrom\s*|\bimport\s*)['"]echarts['"]/;
  const offenders = [];

  for (const file of files) {
    if (fullImport.test(await readFile(file, 'utf8'))) offenders.push(file);
  }

  assert.deepEqual(offenders, []);
});

test('the shared ECharts runtime renders every supported chart type', () => {
  const runtimeMessages = [];
  const originalWarn = console.warn;
  const originalError = console.error;
  console.warn = (...args) => runtimeMessages.push(args.join(' '));
  console.error = (...args) => runtimeMessages.push(args.join(' '));

  let chart;

  try {
    chart = init(null, null, {
      renderer: 'svg',
      ssr: true,
      width: 800,
      height: 600,
    });
    chart.setOption({
      tooltip: { trigger: 'axis' },
      legend: {},
      grid: {
        outerBoundsMode: 'same',
        outerBoundsContain: 'axisLabel',
      },
      dataZoom: [{ type: 'inside' }],
      xAxis: { type: 'category', data: ['0', '1'] },
      yAxis: { type: 'value' },
      series: [
        { name: 'line', type: 'line', data: [1, 2] },
        { name: 'bar', type: 'bar', data: [2, 1] },
        {
          name: 'pie',
          type: 'pie',
          center: ['75%', '25%'],
          radius: 40,
          labelLayout: { hideOverlap: true },
          data: [{ name: 'slice', value: 1 }],
        },
      ],
    });

    const svg = chart.renderToSVGString();
    assert.match(svg, /^<svg\b/);
    assert.deepEqual(
      chart.getOption().series.map((series) => series.type),
      ['line', 'bar', 'pie'],
    );
  } finally {
    chart?.dispose();
    console.warn = originalWarn;
    console.error = originalError;
  }

  assert.deepEqual(runtimeMessages, []);
});

test('range charts isolate a single zoomed timestamp and restore samples and gaps', () => {
  const points = Array.from({ length: 40 }, (_, index) => ({
    timestamp: 1_700_000_000_000 + index * 30_000,
    value: index === 20 ? null : index % 6 < 3 ? 0 : 100,
  }));
  const allocation = points.map((point) => ({
    ...point,
    value: point.value === null ? null : 50,
  }));
  const chart = init(null, null, {
    renderer: 'svg',
    ssr: true,
    width: 800,
    height: 600,
  });
  let tooltipPayload;
  chart.on('showTip', (payload) => {
    tooltipPayload = payload;
  });

  try {
    const options = buildTimeSeriesOptions({
      series: [
        { name: 'allocation', data: allocation },
        { name: 'usage', data: points },
      ],
    });
    chart.setOption(options);

    // A one-point zoom must neither pull nearby samples onto the same pixel
    // nor turn a missing sample into a zero-valued line.
    for (const index of [0, 19, 20, 39]) {
      chart.dispatchAction({
        type: 'dataZoom',
        startValue: points[index].timestamp,
        endValue: points[index].timestamp,
      });
      for (const series of chart.getModel().getSeries()) {
        const data = series.getData();
        assert.equal(data.count(), 1);
        assert.equal(data.getRawIndex(0), index);
        const layout = data.getLayout('points');
        assert.equal(layout.length, 2);
        assert.equal(Number.isFinite(layout[1]), index !== 20);
      }

      const firstSeries = chart.getModel().getSeriesByIndex(0);
      const plot = firstSeries.coordinateSystem.master.getRect();
      tooltipPayload = undefined;
      chart.dispatchAction({
        type: 'updateAxisPointer',
        x: chart.convertToPixel({ xAxisIndex: 0 }, points[index].timestamp),
        y: plot.y + plot.height / 2,
      });
      const tooltipRows = tooltipPayload?.dataByCoordSys?.flatMap((coordinate) =>
        coordinate.dataByAxis.flatMap((axis) => axis.seriesDataIndices),
      );
      assert.deepEqual(
        tooltipRows?.map(({ seriesIndex, dataIndex }) => [seriesIndex, dataIndex]),
        [[0, index], [1, index]],
      );
    }

    chart.dispatchAction({ type: 'dataZoom', startValue: points[18].timestamp, endValue: points[22].timestamp });
    chart.resize({ width: 320, height: 600 });
    chart.setOption(layoutTimeSeriesOptions(options, 320, [points[18].timestamp, points[22].timestamp]));
    for (const series of chart.getModel().getSeries()) {
      const data = series.getData();
      assert.equal(data.count(), 5);
      assert.equal(data.getRawIndex(0), 18);
      assert.equal(data.getRawIndex(4), 22);
    }

    chart.dispatchAction({ type: 'dataZoom', start: 0, end: 100 });
    for (const [seriesIndex, expected] of [allocation, points].entries()) {
      const data = chart.getModel().getSeriesByIndex(seriesIndex).getData();
      assert.equal(data.count(), expected.length);
      expected.forEach(({ value }, index) => {
        assert.equal(data.getRawIndex(index), index);
        const actual = data.get(data.mapDimension('y'), index);
        assert.equal(actual, value === null ? NaN : value);
      });
      const layout = data.getLayout('points');
      assert.ok(Number.isFinite(layout[19 * 2 + 1]));
      assert.ok(Number.isNaN(layout[20 * 2 + 1]));
      assert.ok(Number.isFinite(layout[21 * 2 + 1]));
    }
  } finally {
    chart.dispose();
  }
});
