import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';

import { SVGRenderer } from 'echarts/renderers';

import { init } from '../src/plugins/echarts.mjs';
import { use } from 'echarts/core';

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
