import { formatQueryTimestamp } from '../../../../src/utils/query-time.mjs';

const interval = 30_000;

export function startVisibleSnapshots(readers, initialEnd, page = document) {
  let timer;
  let active;
  let stopped = false;

  async function refresh(at = Date.now()) {
    if (stopped || page.hidden || active) return;
    clearTimeout(timer);
    const controller = new AbortController();
    active = controller;
    const time = formatQueryTimestamp(at);
    const started = Date.now();
    try {
      await Promise.allSettled(Array.from(readers, async read => read(time, controller.signal)));
    } finally {
      if (active === controller) {
        active = undefined;
        const elapsed = Date.now() - started;
        timer = setTimeout(refresh, elapsed < interval ? interval - elapsed : interval);
      }
    }
  }

  function cancel() {
    clearTimeout(timer);
    active?.abort();
    active = undefined;
  }
  function visibilityChanged() {
    cancel();
    if (!page.hidden) void refresh();
  }
  page.addEventListener('visibilitychange', visibilityChanged);
  void refresh(initialEnd);
  return {
    refresh(at) {
      // A user refresh can advance the trend endpoint during a slow poll.
      // Discard that older round so its late responses cannot replace this one.
      cancel();
      return refresh(at);
    },
    stop() {
      stopped = true;
      page.removeEventListener('visibilitychange', visibilityChanged);
      cancel();
    },
  };
}
