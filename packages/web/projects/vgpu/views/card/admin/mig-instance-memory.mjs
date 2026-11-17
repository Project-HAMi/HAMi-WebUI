import { promQLStringLiteral } from '../../../metrics/promql-template.mjs';

export function migInstanceMemoryQuery(uuid) {
  // A GI can be assigned to a Pod. Its Pod name alone says nothing about the memory scope.
  const scope = `UUID=${promQLStringLiteral(uuid)},GPU_I_ID!="",GPU_CI_ID=""`;
  return `{__name__=~"DCGM_FI_DEV_FB_(USED|FREE)(_BYTES)?",${scope}}`;
}

export function sumMigInstanceMemory(samples, uuid) {
  const instances = new Map();
  let source;
  for (const { metric, value } of samples) {
    const id = metric.GPU_I_ID;
    const field = /^DCGM_FI_DEV_FB_(USED|FREE)(_BYTES)?$/.exec(metric.__name__);
    if (!field || !id || metric.GPU_CI_ID || metric.UUID !== uuid) continue;
    const target = JSON.stringify([metric.cluster, metric.job, metric.instance]);
    if (source !== undefined && source !== target) return null;
    source = target;
    if (!instances.has(id)) instances.set(id, {});
    const instance = instances.get(id);
    const key = field[1];
    // DCGM emits a separate FB_USED with Pod UID for each process-attributed Pod.
    // FREE stays instance-wide even when DCGM copies it onto multiple Pods.
    if (key === 'USED' && (metric.pod_uid || metric.exported_pod_uid || metric.vgpu || metric.exported_vgpu)) continue;
    if (!Number.isFinite(value) || value < 0) return null;
    const mib = field[2] ? value / (1024 * 1024) : value;
    if (instance[key] !== undefined && instance[key] !== mib) return null;
    instance[key] = mib;
  }
  if (!instances.size) return null;
  let used = 0;
  let free = 0;
  let hasCapacity = true;
  for (const instance of instances.values()) {
    if (instance.USED === undefined) return null;
    used += instance.USED;
    if (instance.FREE === undefined) hasCapacity = false;
    else free += instance.FREE;
  }
  const total = hasCapacity ? used + free : undefined;
  return {
    usedGiB: used / 1024,
    totalGiB: hasCapacity ? total / 1024 : undefined,
    percent: total > 0 ? 100 * used / total : undefined,
  };
}
