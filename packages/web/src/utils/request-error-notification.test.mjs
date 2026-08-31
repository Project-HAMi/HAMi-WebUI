import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createRequestErrorNotificationGate,
  requestErrorFingerprint,
} from './request-error-notification.mjs';

const domainError = (overrides = {}) => ({
  kind: 'domain',
  config: {
    method: 'post',
    url: '/api/vgpu/v1/monitor/query/instant-vector',
    data: { query: 'up' },
  },
  status: 523,
  code: 523,
  reason: 'VGPU_DOMAIN_ERROR',
  message: 'Prometheus unavailable',
  ...overrides,
});

test('request error fingerprints ignore query bodies and URL query strings', () => {
  const first = domainError();
  const duplicate = domainError({
    config: {
      ...first.config,
      url: `${first.config.url}?request=second`,
      data: { query: 'a different PromQL expression' },
    },
  });

  assert.equal(requestErrorFingerprint(first), requestErrorFingerprint(duplicate));
});

test('duplicate request errors share a fixed notification window', () => {
  let timestamp = 1000;
  const shouldNotify = createRequestErrorNotificationGate({
    now: () => timestamp,
    windowMs: 5000,
  });

  assert.equal(shouldNotify(domainError()), true);
  timestamp = 4000;
  assert.equal(shouldNotify(domainError()), false);
  timestamp = 5999;
  assert.equal(shouldNotify(domainError()), false);
  timestamp = 6000;
  assert.equal(shouldNotify(domainError()), true);
});

test('materially different request errors remain visible', () => {
  const variants = [
    domainError(),
    domainError({ kind: 'transport' }),
    domainError({ config: { method: 'get', url: '/api/vgpu/v1/node' } }),
    domainError({ status: 502 }),
    domainError({ code: 'ECONNABORTED' }),
    domainError({ reason: 'OTHER_REASON' }),
    domainError({ message: 'A different failure' }),
  ];
  const shouldNotify = createRequestErrorNotificationGate({ now: () => 1000 });

  assert.deepEqual(variants.map(shouldNotify), variants.map(() => true));
});

test('the notification fingerprint cache stays bounded', () => {
  const shouldNotify = createRequestErrorNotificationGate({
    now: () => 1000,
    maxEntries: 2,
  });

  assert.equal(shouldNotify(domainError({ message: 'first' })), true);
  assert.equal(shouldNotify(domainError({ message: 'second' })), true);
  assert.equal(shouldNotify(domainError({ message: 'third' })), true);
  assert.equal(shouldNotify(domainError({ message: 'first' })), true);
});
