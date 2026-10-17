import assert from 'node:assert/strict';
import test from 'node:test';
import { createOptionFilter, limitOptions, matchesKeywords, parseKeywords } from './keyword-search.mjs';

test('keywords are trimmed, split on spaces and lower-cased', () => {
  assert.deepEqual(parseKeywords('  A100-SXM4   node_1 '), ['a100-sxm4', 'node_1']);
  assert.deepEqual(parseKeywords('   '), []);
  assert.deepEqual(parseKeywords(undefined), []);
});

test('every keyword must match within one field, ignoring case and field separators', () => {
  const fields = ['nvidia-a100-001', '198.19.178.165', 'NVIDIA-A100-SXM4-80GB'];
  assert.equal(matchesKeywords(parseKeywords('A100'), fields), true);
  assert.equal(matchesKeywords(parseKeywords('178.16'), fields), true);
  assert.equal(matchesKeywords(parseKeywords('a100sxm4 001'), fields), true);
  assert.equal(matchesKeywords(parseKeywords('a100 h100'), fields), false);
  assert.equal(matchesKeywords(parseKeywords(''), fields), true);
  // Keywords are not matched across the boundary between two fields.
  assert.equal(matchesKeywords(parseKeywords('001198'), fields), false);
});

test('separators typed in a keyword are literal', () => {
  assert.equal(matchesKeywords(parseKeywords('node-1'), ['node1']), false);
  assert.equal(matchesKeywords(parseKeywords('node1'), ['node-1']), true);
  assert.equal(matchesKeywords(parseKeywords('--'), ['node-1', 'gpu_2']), false);
  assert.equal(matchesKeywords(parseKeywords('fe80::1'), ['fe80::1']), true);
});

test('device UUID fragments match with or without dashes', () => {
  const fields = ['GPU-f95362fb-79f5-57bb-9972-86156cd5f525'];
  assert.equal(matchesKeywords(parseKeywords('f95362fb-79f5'), fields), true);
  assert.equal(matchesKeywords(parseKeywords('F95362FB79F5'), fields), true);
  assert.equal(matchesKeywords(parseKeywords('79f5-f953'), fields), false);
});

test('option filters search the label and the extra fields of that option', () => {
  const ips = new Map([['nvidia-a100-001', '198.19.178.165']]);
  const filter = createOptionFilter((value) => [ips.get(value)]);
  assert.equal(filter('178.165', { label: 'nvidia-a100-001', value: 'nvidia-a100-001' }), true);
  assert.equal(filter('A100', { label: 'nvidia-a100-001', value: 'nvidia-a100-001' }), true);
  assert.equal(filter('h100', { label: 'nvidia-a100-001', value: 'nvidia-a100-001' }), false);
  assert.equal(filter('a100', { label: 'All nodes', value: undefined }), false);
});

test('long option lists keep only the first matches and count the rest', () => {
  const options = [{ label: 'All', value: '' }, ...Array.from({ length: 250 }, (_, index) => ({ label: `node-${index}`, value: `node-${index}` }))];
  const ips = (value) => [value === 'node-7' ? '10.9.9.9' : undefined];
  assert.deepEqual([limitOptions(options, '').options.length, limitOptions(options, '').hidden], [100, 151]);
  assert.equal(limitOptions(options, '').options[0].label, 'All');
  assert.deepEqual(limitOptions(options, 'node-24').options.map((option) => option.value), ['node-24', 'node-240', 'node-241', 'node-242', 'node-243', 'node-244', 'node-245', 'node-246', 'node-247', 'node-248', 'node-249']);
  assert.equal(limitOptions(options, 'node-24').hidden, 0);
  assert.deepEqual(limitOptions(options, '9.9', ips).options.map((option) => option.value), ['node-7']);
  assert.deepEqual(limitOptions(options, 'node', ips, 5), { options: options.slice(1, 6), hidden: 245 });
});
