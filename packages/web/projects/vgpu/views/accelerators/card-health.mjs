export const healthLabels = { healthy: 'card.normal', unhealthy: 'card.abnormal', unknown: 'card.unknown' };
const icons = { healthy: 'status-schedulable', unhealthy: 'status-unschedulable', unknown: 'status-unmanaged' };

export function cardHealth({ health, isExternal, physicalKey }) {
  if ((isExternal && !physicalKey) || typeof health !== 'boolean') return 'unknown';
  return health ? 'healthy' : 'unhealthy';
}

export function cardHealthDisplay(card, t) {
  const key = cardHealth(card);
  return { icon: icons[key], text: t(healthLabels[key]) };
}
