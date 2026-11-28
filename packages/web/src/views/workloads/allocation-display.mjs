export const ALLOCATION_SHAPES = ['whole', 'template', 'soft', 'mig', 'unknown'];
export const CORES_UNKNOWN_REASONS = [
  'catalog_unavailable',
  'model_not_configured',
  'template_not_configured',
  'compute_not_configured',
  'mode_ambiguous',
  'node_mode_unknown',
];

// undefined for allocations the server does not interpret.
export const getAllocationShapeCopy = ({ allocationShape, template } = {}) => {
  if (!ALLOCATION_SHAPES.includes(allocationShape)) return undefined;
  if (template && (allocationShape === 'template' || allocationShape === 'mig')) {
    return { key: `task.allocation.shape.${allocationShape}Named`, params: { template } };
  }
  return { key: `task.allocation.shape.${allocationShape}`, params: {} };
};

// A soft-split share of 0 reserves nothing; the runtime then shares the NPU by default priority.
export const isUnreservedSoftSplit = ({ allocationShape, allocatedCores, allocatedCoresKnown } = {}) => (
  allocationShape === 'soft' && allocatedCoresKnown !== false && !Number(allocatedCores)
);

export const SHAPE_UNKNOWN_REASONS = [
  'mig_reservation_invalid',
  'mig_reservation_missing',
  'mig_reservation_mismatch',
  'device_mode_unknown',
];

// Why the shape is unknown: the server's own reason, or for Ascend the
// compute-share reason, since there both come from the same mode question.
export const getShapeUnknownReasonKey = (row = {}) => {
  if (row.allocationShape !== 'unknown') return '';
  if (SHAPE_UNKNOWN_REASONS.includes(row.allocationShapeReason)) {
    return `task.allocation.shapeReason.${row.allocationShapeReason}`;
  }
  return getCoresUnknownReasonKey(row);
};

export const getCoresUnknownReasonKey = ({ allocatedCoresKnown, allocatedCoresReason } = {}) => (
  allocatedCoresKnown === false && CORES_UNKNOWN_REASONS.includes(allocatedCoresReason)
    ? `task.allocation.reason.${allocatedCoresReason}`
    : ''
);

// Beside the split, the compute reason is left out when it is the reason the split already shows.
export const getCoresOnlyReasonKey = (row = {}) => {
  const key = getCoresUnknownReasonKey(row);
  return key && key === getShapeUnknownReasonKey(row) ? '' : key;
};
