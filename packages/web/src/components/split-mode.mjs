// How HAMi divides a device, as its device plugins register it.
export const SPLIT_MODES = ['hami-core', 'mig', 'template'];

// MIG and Ascend vNPU templates partition the hardware; HAMi-core shares it.
const ICONS = Object.freeze({
  mig: 'split-partition',
  template: 'split-partition',
  'hami-core': 'split-hami-core',
  soft: 'split-hami-core',
  whole: 'vgpu-card',
});

export const getSplitModeKey = (mode) => (SPLIT_MODES.includes(mode) ? `card.splitMode.${mode}` : '');

export const getSplitIcon = (modeOrShape) => ICONS[modeOrShape] || '';
