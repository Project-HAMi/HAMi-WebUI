const positive = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
};

const index = (value) => {
  const number = Number(value);
  return Number.isInteger(number) && number >= 0 ? number : undefined;
};

// Far beyond any GPU's placement space; a larger extent is a corrupt record, not a layout.
const MAX_SLOTS = 32;
const placementOf = (start, size) => (size > 0 && start !== undefined && start + size <= MAX_SLOTS
  ? { start, size }
  : undefined);

const workloadOf = (container) => ({
  podUid: container.podUid,
  container: container.name,
  appName: container.appName,
  namespace: container.namespace,
});

const isCurrent = (workload, highlight) => Boolean(
  highlight?.podUid && highlight?.container
    && workload.podUid === highlight.podUid
    && workload.container === highlight.container,
);

// One entry per allocation on the device; a container can hold two MIG
// instances of one GPU.
export const collectHolders = (device = {}, containers = [], highlight) => {
  const holders = [];
  for (const container of containers) {
    (container?.devices || []).forEach((allocated, position) => {
      if (!allocated?.id || allocated.id !== device.uuid) return;
      const workload = workloadOf(container);
      const start = index(allocated.migStart);
      const size = positive(allocated.migSize);
      holders.push({
        key: `${workload.podUid}/${workload.container}/${position}`,
        shape: allocated.allocationShape || '',
        name: allocated.template || '',
        memoryMiB: positive(allocated.allocatedMem),
        cores: positive(allocated.allocatedCores),
        coresKnown: allocated.allocatedCoresKnown !== false,
        shapeReason: allocated.allocationShapeReason || '',
        coresReason: allocated.allocatedCoresReason || '',
        placement: placementOf(start, size),
        workload,
        current: isCurrent(workload, highlight),
      });
    });
  }
  return holders;
};

const migProfiles = (device) => (device.migProfiles || [])
  .map((profile) => ({
    name: profile?.name || '',
    placements: (profile?.placements || [])
      .map((placement) => placementOf(index(placement?.start), positive(placement?.size)))
      .filter(Boolean),
  }))
  .filter((profile) => profile.name && profile.placements.length);

const covers = (placement) => Array.from({ length: placement.size }, (_, offset) => placement.start + offset);

// How many more instances of each profile fit: the most non-overlapping
// allowed placements that avoid every used slot.
const fitting = (profiles, occupied) => profiles.map((profile) => {
  const open = profile.placements
    .filter((placement) => covers(placement).every((slot) => !occupied[slot]))
    .sort((left, right) => (left.start + left.size) - (right.start + right.size));
  let end = 0;
  let count = 0;
  for (const placement of open) {
    if (placement.start >= end) {
      count += 1;
      end = placement.start + placement.size;
    }
  }
  return { name: profile.name, count, open };
});

// MIG devices are drawn in the placement space the plugin registered, one
// lane per set of instances that do not overlap.
export const buildMigSplit = (device, holders) => {
  const profiles = migProfiles(device);
  const registered = profiles.reduce((widest, profile) => profile.placements.reduce(
    (edge, placement) => Math.max(edge, placement.start + placement.size), widest,
  ), 0);
  const reach = holders.reduce((edge, holder) => Math.max(edge, holder.placement.start + holder.placement.size), 0);
  const slots = Math.max(registered, reach);
  const laneEnds = [];
  const blocks = [...holders]
    .sort((left, right) => left.placement.start - right.placement.start)
    .map((holder) => {
      let lane = laneEnds.findIndex((end) => end <= holder.placement.start);
      if (lane < 0) {
        lane = laneEnds.length;
        laneEnds.push(0);
      }
      laneEnds[lane] = holder.placement.start + holder.placement.size;
      return { ...holder, lane };
    });
  const occupied = Array(slots).fill(false);
  blocks.forEach((block) => covers(block.placement).forEach((slot) => { occupied[slot] = true; }));
  const fits = fitting(profiles, occupied);
  const usable = Array(slots).fill(false);
  fits.forEach((fit) => fit.open.forEach((placement) => covers(placement).forEach((slot) => { usable[slot] = true; })));
  return {
    kind: 'mig',
    slots,
    lanes: Math.max(laneEnds.length, 1),
    blocks,
    // The rows read in the order the slices are drawn.
    holders: blocks,
    // Without registered profiles, nothing tells whether a free slot can take an instance.
    cells: occupied.map((used, slot) => {
      if (used) return { slot, state: 'used' };
      if (!profiles.length) return { slot, state: 'unknown' };
      return { slot, state: usable[slot] ? 'open' : 'stranded' };
    }),
    fits: profiles.length
      ? fits.filter((fit) => fit.count > 0).map(({ name, count }) => ({ name, count }))
      : undefined,
    overlapping: laneEnds.length > 1,
    beyondRegistered: registered > 0 && reach > registered,
  };
};

// Templates and whole cards are fixed parts of the device memory.
export const buildMemorySplit = (device, holders) => {
  const total = positive(device.memoryTotal);
  const used = holders.reduce((sum, holder) => sum + holder.memoryMiB, 0);
  const scale = Math.max(total, used);
  return {
    kind: 'memory',
    total,
    used,
    free: Math.max(total - used, 0),
    over: total ? Math.max(used - total, 0) : 0,
    blocks: holders.map((holder) => ({ ...holder, share: scale ? holder.memoryMiB / scale : 0 })),
  };
};

const meter = (total, parts) => {
  const used = parts.reduce((sum, part) => sum + part.value, 0);
  const scale = Math.max(total, used);
  return {
    total,
    used,
    over: total ? Math.max(used - total, 0) : 0,
    parts: parts.map((part) => ({ ...part, share: scale ? part.value / scale : 0 })),
  };
};

// HAMi-core shares the whole device: each holder has a memory and a compute
// quota, and nothing is carved out.
export const buildSharedSplit = (device, holders) => ({
  kind: 'shared',
  memory: meter(positive(device.memoryTotal), holders.map((holder) => ({
    key: holder.key, value: holder.memoryMiB, current: holder.current,
  }))),
  compute: meter(positive(device.coreTotal) || 100, holders
    .filter((holder) => holder.coresKnown && holder.cores > 0)
    .map((holder) => ({ key: holder.key, value: holder.cores, current: holder.current }))),
  unlimited: holders.filter((holder) => holder.coresKnown && holder.cores === 0).length,
  holderCount: holders.length,
  limit: positive(device.vgpuTotal),
});

export const splitKind = (device, holders) => {
  const placed = holders.every((holder) => holder.placement);
  if ((device.mode === 'mig' || holders.some((holder) => holder.shape === 'mig')) && placed) return 'mig';
  const shared = holders.every((holder) => holder.shape === 'soft');
  if (shared && (holders.length > 0 || device.mode === 'hami-core')) return 'shared';
  return 'memory';
};

export const buildDeviceSplit = ({ device = {}, containers = [], highlight } = {}) => {
  const holders = collectHolders(device, containers, highlight);
  const kind = splitKind(device, holders);
  const split = {
    mig: buildMigSplit,
    shared: buildSharedSplit,
    memory: buildMemorySplit,
  }[kind](device, holders);
  return { holders, ...split };
};
