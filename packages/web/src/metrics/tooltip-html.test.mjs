import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildPieTooltipFormatter,
  buildTimeSeriesTooltipFormatter,
  escapeTooltipHtmlText,
} from './tooltip-html.mjs';

test('card type tooltip treats device metadata as text', () => {
  const deviceType =
    'Ascend910B<img src=x onerror="globalThis.compromised=true">';
  const tooltip = buildPieTooltipFormatter({ unit: 'cards' })({
    name: deviceType,
    value: 2,
  });

  assert.equal(
    tooltip,
    'Ascend910B&lt;img src=x onerror=&quot;globalThis.compromised=true&quot;&gt;: 2 cards',
  );
  assert.doesNotMatch(tooltip, /<\s*(?:img|script)\b/i);
});

test('tooltip text escapes every HTML-significant character', () => {
  assert.equal(
    escapeTooltipHtmlText(`A&B <C> "D" 'E'`),
    'A&amp;B &lt;C&gt; &quot;D&quot; &#39;E&#39;',
  );
});

test('normal card type tooltips keep their existing presentation', () => {
  assert.equal(
    buildPieTooltipFormatter({ unit: '张' })({ name: 'Ascend910B', value: 2 }),
    'Ascend910B: 2 张',
  );
  assert.equal(
    buildPieTooltipFormatter()({ name: 'NVIDIA A100', value: 1 }),
    'NVIDIA A100: 1',
  );
});

test('every series tooltip escapes the names a cluster supplies', () => {
  const formatter = buildTimeSeriesTooltipFormatter({ digits: 1, unit: '%' });
  const tooltip = formatter([
    {
      axisValueLabel: '12:00<script>',
      seriesName: 'node-a"<img src=x onerror="globalThis.compromised=true">',
      color: '#5B8FF9',
      value: 42.125,
    },
  ]);

  assert.doesNotMatch(tooltip, /<\s*(?:img|script)\b/i);
  assert.match(tooltip, /12:00&lt;script&gt;/);
  assert.match(tooltip, /node-a&quot;&lt;img/);
  assert.match(tooltip, /42\.1 %/);
  assert.equal(formatter([]), '');
});
