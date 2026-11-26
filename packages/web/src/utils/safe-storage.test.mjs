import assert from 'node:assert/strict';
import test from 'node:test';

import { createSafeStorage } from './safe-storage.mjs';

test('safeStorage namespaced reads and writes JSON without throwing', () => {
  const backing = new Map();
  const mockStorage = {
    getItem: (k) => (backing.has(k) ? backing.get(k) : null),
    setItem: (k, v) => backing.set(k, v),
    removeItem: (k) => backing.delete(k),
  };
  const storage = createSafeStorage(mockStorage);

  assert.equal(storage.setJson('route-history', [{ path: '/nodes' }]), true);
  assert.ok(backing.has('hami-webui:route-history'));
  assert.deepEqual(storage.getJson('route-history', []), [{ path: '/nodes' }]);

  backing.set('hami-webui:route-history', '{corrupt-json');
  assert.deepEqual(storage.getJson('route-history', []), []);
});

test('safeStorage falls back to memory when host localStorage throws SecurityError', () => {
  const throwingStorage = {
    getItem: () => {
      throw new DOMException('Blocked by sandbox', 'SecurityError');
    },
    setItem: () => {
      throw new DOMException('Blocked by sandbox', 'SecurityError');
    },
    removeItem: () => {
      throw new DOMException('Blocked by sandbox', 'SecurityError');
    },
  };
  const storage = createSafeStorage(throwingStorage);

  assert.equal(storage.setJson('history-test', [{ path: '/accelerators' }]), false);
  assert.deepEqual(storage.getJson('history-test', []), [{ path: '/accelerators' }]);
  storage.removeItem('history-test');
  assert.deepEqual(storage.getJson('history-test', []), []);
});

test('safeStorage reads in-memory updates when host storage is readable but setItem or removeItem throws', () => {
  const backing = new Map([['hami-webui:route-history', JSON.stringify([{ path: '/old' }])]]);
  const quotaStorage = {
    getItem: (k) => (backing.has(k) ? backing.get(k) : null),
    setItem: () => {
      throw new DOMException('Quota exceeded', 'QuotaExceededError');
    },
    removeItem: () => {
      throw new DOMException('Remove blocked', 'SecurityError');
    },
  };
  const storage = createSafeStorage(quotaStorage);

  assert.equal(storage.setJson('route-history', [{ path: '/new' }]), false);
  assert.deepEqual(storage.getJson('route-history', []), [{ path: '/new' }]);

  storage.removeItem('route-history');
  assert.equal(storage.getItem('route-history'), null);
});
