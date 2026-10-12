export const isNpuVendor = (vendor) => vendor === 'Ascend';

// Shared copy names the device GPU; Ascend devices are NPUs. vGPU is left alone.
export const deviceWording = (text, vendor) => (
  isNpuVendor(vendor) && typeof text === 'string' ? text.replace(/\bGPU/g, 'NPU') : text
);

// The vendor shared by every item, or '' when they differ or one is unknown.
export const sharedVendor = (vendors = []) => (
  vendors.length && vendors.every((vendor) => vendor && vendor === vendors[0]) ? vendors[0] : ''
);
