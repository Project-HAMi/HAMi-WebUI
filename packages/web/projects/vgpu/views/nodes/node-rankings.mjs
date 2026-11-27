const asFiniteNonNeg = (value) => {
  if (typeof value !== 'number') return undefined;
  return Number.isFinite(value) && value >= 0 ? value : undefined;
};

function topFiveNodes(nodes, field) {
  const rows = [];
  for (const node of nodes) {
    if (!node || typeof node.name !== 'string' || !node.name) continue;
    const value = asFiniteNonNeg(node[field]);
    if (value === undefined) continue;
    const uid = typeof node.uid === 'string' ? node.uid : '';
    rows.push({
      name: node.name,
      physicalKey: uid || node.name,
      uid,
      value,
    });
  }
  return rows
    .sort((a, b) => b.value - a.value || a.name.localeCompare(b.name) || a.uid.localeCompare(b.uid))
    .slice(0, 5);
}

/**
 * Compute node vendor distribution from physical node providers.
 * @param {Array<{ providers?: string[] }>} nodes
 */
export function buildNodeVendorDistribution(nodes = []) {
  const counts = new Map();
  for (const node of Array.isArray(nodes) ? nodes : []) {
    const uniqueProviders = new Set(
      (Array.isArray(node?.providers) ? node.providers : [])
        .map((provider) => (typeof provider === 'string' ? provider.trim() : ''))
        .filter(Boolean),
    );
    for (const provider of uniqueProviders) {
      counts.set(provider, (counts.get(provider) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .map(([name, value]) => ({ name, vendor: name, value }))
    .sort((a, b) => b.value - a.value || a.name.localeCompare(b.name));
}

/**
 * Build top-5 node ranking series from NodeItem allocation and usage percentages.
 * @param {Array<object>} nodes
 */
export function buildNodeRankings(nodes = []) {
  const list = Array.isArray(nodes) ? nodes : [];
  return {
    computeAllocation: topFiveNodes(list, 'coreAllocationPercent'),
    computeUsage: topFiveNodes(list, 'computeUtilizationPercent'),
    memoryAllocation: topFiveNodes(list, 'memoryAllocationPercent'),
    memoryUsage: topFiveNodes(list, 'memoryUsagePercent'),
  };
}
