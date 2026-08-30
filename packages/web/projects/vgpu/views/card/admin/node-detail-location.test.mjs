import assert from 'node:assert/strict';
import test from 'node:test';

import { buildNodeDetailLocation } from './node-detail-location.mjs';

test('node detail location uses the named route and normalized identity', () => {
  assert.deepEqual(
    buildNodeDetailLocation({ uid: '  node-uid  ', nodeName: '  worker-1  ' }),
    {
      name: 'node-admin-detail',
      params: { uid: 'node-uid' },
      query: { nodeName: 'worker-1' },
    },
  );
});

test('node detail location omits an empty display-name query', () => {
  assert.deepEqual(buildNodeDetailLocation({ uid: 'node-uid', nodeName: '  ' }), {
    name: 'node-admin-detail',
    params: { uid: 'node-uid' },
  });
});

test('node detail location rejects identities that cannot be navigated', () => {
  assert.equal(buildNodeDetailLocation(), null);
  assert.equal(buildNodeDetailLocation({ uid: '' }), null);
  assert.equal(buildNodeDetailLocation({ uid: '   ' }), null);
  assert.equal(buildNodeDetailLocation({ uid: 42 }), null);
});
