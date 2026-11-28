import assert from 'node:assert/strict';
import test from 'node:test';

import {
  CHUNK_RELOAD_STORAGE_KEY,
  installChunkRecovery,
  isChunkLoadError,
} from './chunk-recovery.mjs';

function createMemoryStorage() {
  const store = new Map();
  return {
    getItem(key) {
      return store.has(key) ? store.get(key) : null;
    },
    setItem(key, value) {
      store.set(key, String(value));
    },
    removeItem(key) {
      store.delete(key);
    },
  };
}

function createFakeWindow({ href = 'https://webui.example/gpu-ui/overview', storage } = {}) {
  const listeners = new Map();
  const navigated = [];
  const win = {
    location: {
      href,
      origin: new URL(href).origin,
      assign(nextHref) {
        navigated.push(nextHref);
      },
    },
    sessionStorage: storage ?? createMemoryStorage(),
    addEventListener(type, fn) {
      if (!listeners.has(type)) listeners.set(type, new Set());
      listeners.get(type).add(fn);
    },
    removeEventListener(type, fn) {
      listeners.get(type)?.delete(fn);
    },
    dispatchEvent(event) {
      for (const fn of listeners.get(event.type) ?? []) {
        fn(event);
      }
    },
  };
  return { win, navigated, listeners };
}

function createFakeRouter(basePath = '/gpu-ui/') {
  const beforeHooks = new Set();
  const afterHooks = new Set();
  const errorHooks = new Set();

  return {
    beforeHooks,
    afterHooks,
    errorHooks,
    resolve(to) {
      const normalizedBase = basePath.endsWith('/') ? basePath.slice(0, -1) : basePath;
      const fullPath = to.fullPath.startsWith('/') ? to.fullPath : `/${to.fullPath}`;
      return { href: `${normalizedBase}${fullPath}` };
    },
    beforeEach(fn) {
      beforeHooks.add(fn);
      return () => beforeHooks.delete(fn);
    },
    afterEach(fn) {
      afterHooks.add(fn);
      return () => afterHooks.delete(fn);
    },
    onError(fn) {
      errorHooks.add(fn);
      return () => errorHooks.delete(fn);
    },
    emitBeforeEach(to, from = {}) {
      for (const fn of beforeHooks) fn(to, from);
    },
    emitAfterEach(to, from = {}, failure = undefined) {
      for (const fn of afterHooks) fn(to, from, failure);
    },
    emitError(error, to) {
      for (const fn of errorHooks) fn(error, to);
    },
  };
}

test('isChunkLoadError identifies chunk/preload failures and rejects ordinary errors', () => {
  assert.equal(isChunkLoadError(null), false);
  assert.equal(isChunkLoadError(undefined), false);
  assert.equal(isChunkLoadError(new Error('Request failed with status 500')), false);
  assert.equal(isChunkLoadError(new TypeError("Cannot read properties of undefined (reading 'id')")), false);

  assert.equal(isChunkLoadError({ name: 'ChunkLoadError', message: 'Loading chunk 12 failed' }), true);
  assert.equal(
    isChunkLoadError(new TypeError('Failed to fetch dynamically imported module: https://host/static/Nodes-123.js')),
    true
  );
  assert.equal(
    isChunkLoadError(new TypeError('Importing a module script failed.')),
    true
  );
  assert.equal(
    isChunkLoadError(new Error('error loading dynamically imported module: https://host/static/Workloads.js')),
    true
  );
  assert.equal(
    isChunkLoadError(new Error('Unable to preload CSS for static/Detail-abc.css')),
    true
  );
  assert.equal(
    isChunkLoadError(new Error('Failed to load module script: Expected a JavaScript module script')),
    true
  );
});

test('router.onError triggers a single recovery reload to resolved basePath URL and blocks loops', () => {
  const { win, navigated } = createFakeWindow();
  const router = createFakeRouter('/gpu-ui/');
  const dispose = installChunkRecovery(router, { win });
  const target = { fullPath: '/nodes?status=healthy#top' };
  const chunkErr = new TypeError('Failed to fetch dynamically imported module: https://webui.example/gpu-ui/static/Nodes.js');

  router.emitBeforeEach(target);
  router.emitError(new Error('unrelated API failure'), target);
  assert.deepEqual(navigated, []);

  router.emitError(chunkErr, target);
  assert.deepEqual(navigated, ['https://webui.example/gpu-ui/nodes?status=healthy#top']);
  assert.equal(
    win.sessionStorage.getItem(CHUNK_RELOAD_STORAGE_KEY),
    'https://webui.example/gpu-ui/nodes?status=healthy#top'
  );

  // Even after a full reload creates a new handler instance, the same failing URL must not loop.
  dispose();
  const second = createFakeRouter('/gpu-ui/');
  const navigatedSecond = [];
  const disposeSecond = installChunkRecovery(second, {
    win: { ...win, location: { ...win.location, href: 'https://webui.example/gpu-ui/nodes?status=healthy#top' } },
    navigate: (href) => navigatedSecond.push(href),
  });
  second.emitBeforeEach(target);
  second.emitError(chunkErr, target);
  assert.deepEqual(navigatedSecond, []);
  disposeSecond();
});

test('vite:preloadError reloads toward pending route target, prevents default, and deduplicates onError', () => {
  const { win, navigated } = createFakeWindow();
  const router = createFakeRouter('/gpu-ui/');
  const dispose = installChunkRecovery(router, { win });
  const target = { fullPath: '/accelerators' };

  router.emitBeforeEach(target);
  let defaultPrevented = false;
  win.dispatchEvent({
    type: 'vite:preloadError',
    preventDefault() {
      defaultPrevented = true;
    },
  });
  // Router onError firing for the same failed dynamic import must not double-navigate.
  router.emitError(new TypeError('Failed to fetch dynamically imported module'), target);

  assert.equal(defaultPrevented, true);
  assert.deepEqual(navigated, ['https://webui.example/gpu-ui/accelerators']);
  dispose();
});

test('successful afterEach clears reload guard, while failed afterEach preserves it', () => {
  const { win, navigated } = createFakeWindow();
  const router = createFakeRouter('/gpu-ui/');
  const dispose = installChunkRecovery(router, { win });
  const target = { fullPath: '/nodes' };
  const chunkErr = new TypeError('Failed to fetch dynamically imported module');

  router.emitBeforeEach(target);
  router.emitError(chunkErr, target);
  assert.equal(navigated.length, 1);

  // Simulated failure hook must not clear the guard.
  router.emitAfterEach(target, {}, new Error('aborted'));
  assert.ok(win.sessionStorage.getItem(CHUNK_RELOAD_STORAGE_KEY));

  // Subsequent fresh session succeeds navigation -> guard clears so future upgrades recover.
  dispose();
  const freshWin = createFakeWindow({
    href: 'https://webui.example/gpu-ui/nodes',
    storage: win.sessionStorage,
  });
  const freshRouter = createFakeRouter('/gpu-ui/');
  const disposeFresh = installChunkRecovery(freshRouter, { win: freshWin.win });
  freshRouter.emitAfterEach(target, {}, undefined);
  assert.equal(freshWin.win.sessionStorage.getItem(CHUNK_RELOAD_STORAGE_KEY), null);

  // Hours later, navigating to /workloads hits a new deploy 404 and can recover once again.
  const workloadsTarget = { fullPath: '/workloads' };
  freshRouter.emitBeforeEach(workloadsTarget);
  freshRouter.emitError(chunkErr, workloadsTarget);
  assert.deepEqual(freshWin.navigated, ['https://webui.example/gpu-ui/workloads']);
  disposeFresh();
});

test('blocked sessionStorage in sandboxed iframe fails closed without throwing or looping', () => {
  const throwingStorage = {
    getItem() {
      throw new DOMException('Blocked', 'SecurityError');
    },
    setItem() {
      throw new DOMException('Blocked', 'SecurityError');
    },
    removeItem() {
      throw new DOMException('Blocked', 'SecurityError');
    },
  };
  const { win, navigated } = createFakeWindow({ storage: throwingStorage });
  const router = createFakeRouter('/gpu-ui/');
  const dispose = installChunkRecovery(router, { win });

  router.emitBeforeEach({ fullPath: '/nodes' });
  router.emitError(new TypeError('Failed to fetch dynamically imported module'), { fullPath: '/nodes' });
  let defaultPrevented = false;
  win.dispatchEvent({
    type: 'vite:preloadError',
    preventDefault() {
      defaultPrevented = true;
    },
  });

  assert.deepEqual(navigated, []);
  assert.equal(defaultPrevented, false);
  dispose();
});

test('dispose unregisters router hooks and window preload listener', () => {
  const { win, listeners } = createFakeWindow();
  const router = createFakeRouter('/gpu-ui/');
  const dispose = installChunkRecovery(router, { win });

  assert.equal(router.beforeHooks.size, 1);
  assert.equal(router.afterHooks.size, 1);
  assert.equal(router.errorHooks.size, 1);
  assert.equal(listeners.get('vite:preloadError')?.size, 1);

  dispose();
  assert.equal(router.beforeHooks.size, 0);
  assert.equal(router.afterHooks.size, 0);
  assert.equal(router.errorHooks.size, 0);
  assert.equal(listeners.get('vite:preloadError')?.size, 0);
});
