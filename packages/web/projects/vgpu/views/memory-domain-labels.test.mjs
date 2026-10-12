import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { LONG_TEXT_TOOLTIP_STYLE } from '../components/tooltip-policy.mjs';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

const nodeDetail = readSource('./node/admin/Detail.vue');
const cardDetail = readSource('./card/admin/Detail.vue');
const metricHelp = readSource('../components/MetricHelp.vue');
const metricChart = readSource('../components/MetricChart.vue');
const enLocale = readSource('../../../src/locales/en.js');
const zhLocale = readSource('../../../src/locales/zh.js');

test('memory labels distinguish schedulable allocation from physical usage', () => {
  assert.match(enLocale, /memAllocRate: 'Schedulable Memory Allocation'/);
  assert.match(enLocale, /memUsageRate: 'Physical Memory Usage'/);
  assert.match(zhLocale, /computeUsageRate: '算力使用率'/);
  assert.match(zhLocale, /memUsageRate: '物理显存使用率'/);
  assert.match(zhLocale, /computeUsageTrend: 'GPU 算力使用率（%）'/);

  for (const source of [nodeDetail, cardDetail]) {
    assert.match(source, /dashboard\.memAllocRateDescription/);
    assert.match(source, /dashboard\.memUsageRateDescription/);
    assert.match(source, /dashboard\.metricHelpLabel/);
  }
});

test('detail memory cards use compact labels without weakening metric help', () => {
  assert.match(
    nodeDetail,
    /dashboard\.allocRateLegend'[\s\S]*?dashboard\.memAllocRateDescription/,
  );
  assert.match(
    nodeDetail,
    /dashboard\.usageRateLegend'[\s\S]*?dashboard\.memUsageRateDescription/,
  );
  assert.match(
    cardDetail,
    /dashboard\.allocated'[\s\S]*?dashboard\.memAllocRateDescription/,
  );
  assert.match(
    cardDetail,
    /dashboard\.used'[\s\S]*?dashboard\.memUsageRateDescription/,
  );
});

test('metric help is available to pointer, keyboard and assistive technology', () => {
  assert.match(metricHelp, /:visible="visible"/);
  assert.match(metricHelp, /@mouseenter="hovered = true"/);
  assert.match(metricHelp, /@focus="focused = true"/);
  assert.match(metricHelp, /@keydown\.esc="dismiss"/);
  assert.match(metricHelp, /hovered\.value = false/);
  assert.match(metricHelp, /focused\.value = false/);
  assert.match(metricHelp, /:aria-describedby="descriptionId"/);
  assert.match(metricHelp, /class="metric-help__description"/);
  assert.match(metricHelp, /LONG_TEXT_TOOLTIP_STYLE/);
  assert.equal(LONG_TEXT_TOOLTIP_STYLE.maxWidth, 'min(320px, calc(100vw - 32px))');
  assert.equal(LONG_TEXT_TOOLTIP_STYLE.overflowWrap, 'anywhere');
});

test('node detail stacks the rates while keeping each rate row horizontal', () => {
  assert.match(
    nodeDetail,
    /\.resource-card-footer\s*\{[^}]*display:\s*flex;[^}]*flex-direction:\s*column;/s,
  );
  assert.match(
    nodeDetail,
    /\.resource-card-rate-wrap\s*\{[^}]*width:\s*100%;[^}]*min-width:\s*0;/s,
  );
  assert.match(
    nodeDetail,
    /\.resource-card-footer-item\s*\{[^}]*display:\s*flex;[^}]*align-items:\s*center;[^}]*justify-content:\s*space-between;/s,
  );
  assert.match(
    nodeDetail,
    /\.resource-card-footer-title\s*\{[^}]*white-space:\s*nowrap;/s,
  );
});

test('memory and compute trend legends use the same compact labels', () => {
  // Both pages build every trend through one helper and the shared preset, so
  // the two charts cannot drift apart.
  for (const source of [nodeDetail, cardDetail]) {
    const helper = source.slice(
      source.indexOf('const trendSection = '),
      source.indexOf('const trendSections'),
    );
    const sections = source.slice(
      source.indexOf('const trendSections'),
      source.indexOf(']);', source.indexOf('const trendSections')),
    );
    assert.match(helper, /name: t\('dashboard\.usageRateLegend'\)/);
    assert.match(helper, /buildTimeSeriesOptions/);
    assert.match(sections, /gpuComputeAllocUsageTrend'\), computeTrend\.value, computeAllocLegend\.value\)/);
    assert.match(sections, /gpuMemAllocUsageTrend'\), memoryTrend\.value, t\('dashboard\.allocRateLegend'\)\)/);
    assert.doesNotMatch(helper + sections, /dashboard\.mem(?:Alloc|Usage)Rate'/);
    // The compute allocation legend switches to its lower-bound form.
    assert.match(source, /'dashboard\.allocRateLowerBoundLegend'\s*:\s*'dashboard\.allocRateLegend'/);
  }
});

test('a detail trend chart draws its lines from one range group', () => {
  for (const source of [cardDetail, nodeDetail]) {
    // Gauges read current values only, so no line comes from a separate request.
    assert.match(source, /_gaugeConfigBase\.map\(\(\{ percentQuery, \.\.\.item \}\)/);
    assert.doesNotMatch(source, /gaugeConfig\[\d\]\??\.data/);
    assert.match(source, /\[\{ query: trendQuery\(0\) \}, \{ query: trendQuery\(2\) \}, \{ query: \w+, optional: true \}\]/);
    assert.match(source, /\[\{ query: trendQuery\(1\) \}, \{ query: trendQuery\(3\) \}\]/);
    assert.match(source, /:refresh-error="Boolean\(section\.refreshError\)"/);
  }
  assert.match(metricChart, /common\.refreshFailedShowingPreviousResult/);
});

test('a trend chart keeps its instance when the language changes', () => {
  const overview = readSource('./monitor/overview/index.vue');
  for (const source of [overview, nodeDetail, cardDetail]) {
    assert.match(source, /v-for="section in (?:rangeConfig|trendSections)"\s+:key="section\.key"/);
  }
  assert.match(cardDetail, /v-for="item in lineToolsView"\s+:key="item\.titleKey"/);
});
