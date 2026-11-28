import { computeAllocationPercent, memoryAllocationPercent } from './allocation-percent.mjs';

const asPercent = ratio => Number.isFinite(ratio) ? ratio * 100 : undefined;

function topFive(cards, percentOf) {
  const rows = [];
  for (const card of cards) {
    const value = percentOf(card);
    if (value !== undefined) {
      rows.push({ name: card.uuid, physicalKey: card.physicalKey, detailTo: card.detailTo, value });
    }
  }
  // The full physical identity breaks ties even when two nodes report the same ID.
  return rows.sort((a, b) => b.value - a.value || a.physicalKey.localeCompare(b.physicalKey)).slice(0, 5);
}

export function physicalCardRankings(cards) {
  return {
    computeAllocation: topFive(cards, card => computeAllocationPercent(card)?.raw),
    computeUsage: topFive(cards, card => asPercent(card.utilizationRatio)),
    memoryAllocation: topFive(cards, card => memoryAllocationPercent(card)?.raw),
    memoryUsage: topFive(cards, card => asPercent(card.memoryUsageRatio)),
  };
}
