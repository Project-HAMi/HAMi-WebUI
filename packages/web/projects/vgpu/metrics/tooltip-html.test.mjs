import assert from 'node:assert/strict';
import test from 'node:test';

import {
  escapeTooltipHtmlText,
  formatCardTypeTooltip,
} from './tooltip-html.mjs';

test('card type tooltip treats device metadata as text', () => {
  const deviceType =
    'Ascend910B<img src=x onerror="globalThis.compromised=true">';
  const tooltip = formatCardTypeTooltip(
    { name: deviceType, value: 2 },
    'cards',
  );

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
    formatCardTypeTooltip({ name: 'Ascend910B', value: 2 }, '张'),
    'Ascend910B: 2 张',
  );
  assert.equal(
    formatCardTypeTooltip({ name: 'NVIDIA A100', value: 1 }, ''),
    'NVIDIA A100: 1',
  );
});
