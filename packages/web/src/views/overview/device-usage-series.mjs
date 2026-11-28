export const binWidth = 10;
const binCount = 100 / binWidth;
export const validRate = (value) => Number.isFinite(value) && value >= 0 && value <= 100;

export function makeUsageSeries(devices, metric, individualDeviceLimit) {
  const missing = [];
  const readings = new Map();
  for (const device of devices) {
    const value = device[metric];
    if (!validRate(value)) {
      missing.push(device);
      continue;
    }
    if (!readings.has(value)) readings.set(value, []);
    readings.get(value).push(device);
  }

  const values = [...readings.keys()].sort((a, b) => a - b);
  const reported = devices.length - missing.length;
  const closeReadings = values.some((value, i) => i > 0 && value - values[i - 1] < binWidth);
  if (values.length > 1 && (reported > individualDeviceLimit || closeReadings)) {
    const bins = Array.from({ length: binCount }, () => []);
    for (const [value, cards] of readings) {
      const index = Math.min(binCount - 1, Math.floor(value / binWidth));
      for (const card of cards) bins[index].push(card);
    }
    return { bins, dots: [], missing };
  }

  const laneEnds = [];
  const dots = values.map(value => {
    const cards = readings.get(value);
    const left = value - (cards.length > 1 && value > 50 ? 36 : 6);
    const right = value + (cards.length > 1 && value <= 50 ? 36 : 6);
    const available = laneEnds.findIndex((end) => end <= left);
    const lane = available < 0 ? laneEnds.length : available;
    laneEnds[lane] = right;
    return { value, cards, lane };
  });
  return { bins: null, dots, lanes: laneEnds.length, missing };
}
