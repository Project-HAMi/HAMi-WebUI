import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  GPU_UUID_TOOLTIP_STYLE,
  LONG_TEXT_TOOLTIP_STYLE,
} from '../components/tooltip-policy.mjs';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

const ellipsisText = readSource('../../../src/components/EllipsisText.vue');
const taskDetail = readSource('./task/admin/Detail.vue');
const cardList = readSource('./card/admin/index.vue');
const overview = readSource('./monitor/overview/index.vue');
const gauge = readSource('../components/gauge.vue');

test('long text tooltips wrap at a consistent readable width', () => {
  assert.deepEqual(LONG_TEXT_TOOLTIP_STYLE, {
    maxWidth: 'min(320px, calc(100vw - 32px))',
    whiteSpace: 'normal',
    overflowWrap: 'anywhere',
    lineHeight: '20px',
  });
  assert.match(ellipsisText, /default: 'vgpu-long-text-tooltip'/);
  assert.match(
    ellipsisText,
    /\.vgpu-long-text-tooltip\s*\{[^}]*max-width:\s*min\(320px, calc\(100vw - 32px\)\);[^}]*overflow-wrap:\s*anywhere;/s,
  );
  assert.match(gauge, /:overlay-inner-style="LONG_TEXT_TOOLTIP_STYLE"/);
  assert.match(taskDetail, /LONG_TEXT_TOOLTIP_STYLE/);
  assert.match(overview, /:overlay-inner-style="LONG_TEXT_TOOLTIP_STYLE"/);
});

test('GPU UUID tooltips remain complete on one line', () => {
  assert.deepEqual(GPU_UUID_TOOLTIP_STYLE, {
    maxWidth: 'none',
    whiteSpace: 'nowrap',
    overflowWrap: 'normal',
  });
  assert.match(
    ellipsisText,
    /\.vgpu-long-text-tooltip\.vgpu-single-line-tooltip\s*\{[^}]*max-width:\s*none;[^}]*white-space:\s*nowrap;/s,
  );
  assert.match(cardList, /tooltipClass="vgpu-long-text-tooltip vgpu-single-line-tooltip"/);
  assert.match(taskDetail, /GPU_UUID_TOOLTIP_STYLE/);
});

test('a cut text is focusable only where a caller opts in', () => {
  assert.match(ellipsisText, /focusable:\s*\{\s*type: Boolean,\s*default: false,/);
  const [tooltipBranch, plainBranch] = ellipsisText.split('<span\n    v-else');
  assert.match(tooltipBranch, /:trigger="focusable \? \['hover', 'focus'\] : 'hover'"/);
  assert.match(tooltipBranch, /:tabindex="focusable \? 0 : undefined"/);
  assert.match(tooltipBranch, /:tabindex="focusable \? 0 : undefined"\s*@keydown\.esc=/);
  assert.doesNotMatch(plainBranch, /tabindex/);
  assert.match(readSource('../components/previewBar.vue'), /<EllipsisText :text="name" focusable \/>/);
});
