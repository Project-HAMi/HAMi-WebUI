// DCU and HCU identify device families from the same vendor.
export const gpuVendor = provider => provider === 'DCU' || provider === 'HCU' ? 'Hygon' : provider;

// Muted vendor hues for the distribution rings.
const palettes = new Map([
  ['NVIDIA', { hue: 88, saturation: 40, lightness: 48 }],
  ['Ascend', { hue: 4, saturation: 54, lightness: 56 }],
  ['MLU', { hue: 212, saturation: 48, lightness: 55 }],
  ['Hygon', { hue: 344, saturation: 39, lightness: 46 }],
  ['Metax', { hue: 279, saturation: 34, lightness: 52 }],
]);
const neutral = { hue: 212, saturation: 9, lightness: 56 };
const paletteFor = provider => palettes.get(gpuVendor(provider)) ?? neutral;

export function donutSliceColors(rows) {
  const groups = new Map();
  for (const { name, vendor } of rows) {
    const palette = paletteFor(vendor);
    if (!groups.has(palette)) groups.set(palette, []);
    groups.get(palette).push(name);
  }

  const colors = new Map();
  for (const [{ hue, saturation, lightness }, names] of groups) {
    // Keep each model's shade stable when counts or the query's row order change.
    names.sort().forEach((name, index) => {
      const offset = names.length === 1 ? 0 : 24 * index / (names.length - 1) - 12;
      colors.set(name, `hsl(${hue}, ${saturation}%, ${Math.round(lightness + offset)}%)`);
    });
  }
  return colors;
}
