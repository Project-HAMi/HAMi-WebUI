// The workload list's own total. Only a well-formed reply counts, so a failed or
// malformed one never reads as zero workloads.
export const readWorkloadTotal = (reply) => {
  const total = reply?.total;
  return Array.isArray(reply?.items) && Number.isSafeInteger(total) && total >= reply.items.length ? total : undefined;
};
