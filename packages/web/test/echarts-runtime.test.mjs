import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';

import { SVGRenderer } from 'echarts/renderers';

import { init } from '../src/plugins/echarts.mjs';
import { use } from 'echarts/core';
import {
  buildRangeDataZoom,
  buildRangeLineSeries,
} from '../projects/vgpu/metrics/range-vector-state.mjs';

use([SVGRenderer]);

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
    timestamp: 1_700_000_000 + index * 30,
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
    chart.setOption({
      animation: false,
      tooltip: { trigger: 'axis' },
      dataZoom: buildRangeDataZoom(),
      xAxis: { type: 'category', data: points.map((point) => point.timestamp) },
      yAxis: { type: 'value' },
      series: [
        buildRangeLineSeries({ name: 'allocation', data: allocation }),
        buildRangeLineSeries({ name: 'usage', data: points }),
      ],
    });

    // With filterMode: 'none', a collapsed category extent maps every sample
    // to the same pixel, producing a vertical line and dozens of tooltip rows.
    // Exercise the real zoom and axis-pointer actions, including an empty sample.
    for (const index of [0, 19, 20, 39]) {
      chart.dispatchAction({
        type: 'dataZoom',
        startValue: index,
        endValue: index,
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
        x: chart.convertToPixel({ xAxisIndex: 0 }, index),
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

    chart.dispatchAction({ type: 'dataZoom', startValue: 18, endValue: 22 });
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
