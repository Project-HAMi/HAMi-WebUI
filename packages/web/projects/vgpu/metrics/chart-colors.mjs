// One palette for every chart. Allocation and usage keep the same two colours
// wherever they are drawn together. Kept apart from the option builders so a
// page can name a colour without pulling in the chart runtime.
export const CHART_COLORS = Object.freeze({
  allocation: '#5B8FF9',
  usage: '#42C090',
  single: '#5B8FF9',
  categorical: Object.freeze([
    '#76B900',
    '#9FCB98',
    '#F59E0B',
    '#4F8F87',
    '#14B8A6',
    '#6B7280',
  ]),
});
