import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

const nodeDetail = readSource('./node/admin/Detail.vue');
const nodeOptions = readSource('./node/admin/getOptions.js');
const cardDetail = readSource('./card/admin/Detail.vue');
const metricHelp = readSource('../components/MetricHelp.vue');
const enLocale = readSource('../../../src/locales/en.js');

test('memory labels distinguish schedulable allocation from physical usage', () => {
  assert.match(enLocale, /memAllocRate: 'Schedulable Memory Allocation'/);
  assert.match(enLocale, /memUsageRate: 'Physical Memory Usage'/);

  for (const source of [nodeDetail, cardDetail]) {
    assert.match(source, /dashboard\.memAllocRateDescription/);
    assert.match(source, /dashboard\.memUsageRateDescription/);
    assert.match(source, /dashboard\.metricHelpLabel/);
  }

  assert.match(cardDetail, /dashboard\.physicalUsed/);
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
});

test('detail footers preserve labels and values at narrow widths', () => {
  for (const source of [nodeDetail, cardDetail]) {
    assert.match(
      source,
      /\.resource-card-footer-item\s*\{[^}]*flex-wrap:\s*wrap;/s,
    );
    assert.match(
      source,
      /\.resource-card-footer-title\s*\{[^}]*flex:\s*1 1 160px;[^}]*min-width:\s*0;/s,
    );
    assert.match(
      source,
      /\.resource-card-footer-value\s*\{[^}]*flex:\s*0 0 auto;[^}]*margin-left:\s*auto;/s,
    );
  }
});

test('memory trends use explicit labels while compute trends keep generic labels', () => {
  assert.match(nodeDetail, /allocationName: t\('dashboard\.memAllocRate'\)/);
  assert.match(nodeDetail, /usageName: t\('dashboard\.memUsageRate'\)/);
  assert.match(
    nodeOptions,
    /name: allocationName \|\| t\('dashboard\.allocRateLegend'\)/,
  );
  assert.match(
    nodeOptions,
    /name: usageName \|\| t\('dashboard\.usageRateLegend'\)/,
  );

  const computeTrend = cardDetail.slice(
    cardDetail.indexOf("dashboard.gpuComputeAllocUsageTrend"),
    cardDetail.indexOf("dashboard.gpuMemAllocUsageTrend"),
  );
  const memoryTrend = cardDetail.slice(
    cardDetail.indexOf("dashboard.gpuMemAllocUsageTrend"),
    cardDetail.indexOf('lineToolsView'),
  );

  assert.match(computeTrend, /dashboard\.allocRateLegend/);
  assert.match(computeTrend, /dashboard\.usageRateLegend/);
  assert.doesNotMatch(computeTrend, /dashboard\.mem(?:Alloc|Usage)Rate/);
  assert.match(memoryTrend, /dashboard\.memAllocRate/);
  assert.match(memoryTrend, /dashboard\.memUsageRate/);
});
