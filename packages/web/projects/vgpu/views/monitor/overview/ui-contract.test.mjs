import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const overview = readFileSync(new URL('./index.vue', import.meta.url), 'utf8');
const styles = readFileSync(new URL('./style.scss', import.meta.url), 'utf8');

test('node overview uses the same combined scheduling eligibility contract', () => {
  assert.match(overview, /isNodeSchedulingEligible/);
  assert.match(overview, /title: t\('node\.schedulable'\)/);
  assert.match(overview, /title: t\('node\.temporarilyUnschedulable'\)/);
  assert.match(overview, /showHelp: temporarilyUnschedulableCount > 0/);
  assert.match(overview, /query: \{ schedulingEligibility: status \}/);
  assert.doesNotMatch(overview, /query: \{ isSchedulable: status \}/);
});

test('resource overview separates numeric values from their units', () => {
  assert.match(overview, /class="count-unit"/);
  assert.match(styles, /\.count-unit\s*\{[^}]*margin-left:\s*4px;/s);
});
