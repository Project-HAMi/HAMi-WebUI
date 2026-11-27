import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { LONG_TEXT_TOOLTIP_STYLE } from '../components/tooltip-policy.mjs';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

const overview = readSource('./monitor/overview/index.vue');
const resourceTrend = readSource('../components/ResourceTrend.vue');
const nodeDetail = readSource('./node/admin/Detail.vue');
const cardDetail = readSource('./card/admin/Detail.vue');
const metricHelp = readSource('../components/MetricHelp.vue');
const metricChart = readSource('../components/MetricChart.vue');
const enLocale = readSource('../../../src/locales/en.js');
const zhLocale = readSource('../../../src/locales/zh.js');

test('memory labels distinguish schedulable allocation from physical usage', () => {
  assert.match(enLocale, /memAllocRate: 'Schedulable Memory Allocation'/);
  assert.match(enLocale, /memUsageRate: 'Physical Memory Usage'/);
  assert.match(zhLocale, /computeUsageRate: '算力利用率'/);
  assert.match(zhLocale, /memUsageRate: '物理显存占用率'/);
  assert.match(zhLocale, /computeUsageTrend: 'GPU 算力利用率（%）'/);

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
    /dashboard\.overviewMemoryOccupancyLegend'[\s\S]*?dashboard\.memUsageRateDescription/,
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

test('overview and details use the same trend card while keeping their own data and allocation coverage', () => {
  assert.match(overview, /<ResourceTrend :kind="section.key" :series="section.series"/);
  for (const source of [nodeDetail, cardDetail]) {
    assert.match(source, /<ResourceTrend kind="compute" :series="computeTrend"/);
    assert.match(source, /<ResourceTrend kind="memory" :series="memoryTrend"/);
  }
  assert.doesNotMatch(nodeDetail, /allocRateLowerBoundLegend/);
  assert.match(cardDetail, /:allocation-label="computeAllocLegend"/);
  assert.match(cardDetail, /'dashboard\.allocRateLowerBoundLegend'\s*:\s*'dashboard\.allocRateLegend'/);
  assert.match(resourceTrend, /note-placement="legend"/);
});

test('a detail trend chart draws its lines from one range group', () => {
  for (const source of [cardDetail, nodeDetail]) {
    assert.doesNotMatch(source, /gaugeConfig\[\d\]\??\.data/);
  }
  assert.match(cardDetail, /_gaugeConfigBase\.map\(\(\{ percentQuery, \.\.\.item \}\)/);
  assert.match(cardDetail, /\[\{ query: trendQuery\(0\) \}, \{ query: trendQuery\(2\) \}, \{ query: \w+, optional: true \}\]/);
  assert.match(cardDetail, /\[\{ query: trendQuery\(1\) \}, \{ query: trendQuery\(3\) \}\]/);
  assert.match(nodeDetail, /useTelemetryTrends\(\{[\s\S]*?scope: 'node'/);
  assert.match(nodeDetail, /const computeTrend = computed\(\(\) => rangeSeries\.value\.filter\(\(item\) => item\.sectionIndex === 'compute'\)\)/);
  assert.match(nodeDetail, /const memoryTrend = computed\(\(\) => rangeSeries\.value\.filter\(\(item\) => item\.sectionIndex === 'memory'\)\)/);
  assert.match(resourceTrend, /:refresh-error=/);
  assert.match(metricChart, /common\.refreshFailedShowingPreviousResult/);
});

test('detail trend keys are independent of the translated chart title', () => {
  for (const source of [nodeDetail, cardDetail]) {
    assert.doesNotMatch(source, /<ResourceTrend[^>]*:key="[^"]*title/i);
  }
  assert.match(cardDetail, /v-for="item in lineToolsView"\s+:key="item\.titleKey"/);
});
