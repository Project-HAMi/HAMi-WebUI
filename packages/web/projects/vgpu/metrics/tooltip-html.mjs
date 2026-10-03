import { formatRangeTooltipValue } from './range-vector-state.mjs';

export const escapeTooltipHtmlText = (value) =>
  String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');

// Series names carry cluster data: node, workload and model names reach a
// tooltip, so every value is escaped before it becomes HTML.
export const buildPieTooltipFormatter = ({ unit = '' } = {}) => ({ name, value }) => {
  const suffix = unit ? ` ${escapeTooltipHtmlText(unit)}` : '';

  return `${escapeTooltipHtmlText(name)}: ${escapeTooltipHtmlText(
    value,
  )}${suffix}`;
};

const tooltipRow = (color, name, value) => `
  <div style="display:flex;align-items:center;font-size:14px;line-height:22px;">
    <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background-color:${escapeTooltipHtmlText(
      color,
    )};margin-right:5px;"></span>
    <span>${escapeTooltipHtmlText(name)}:&nbsp;</span>
    <span style="font-weight:bold;">${escapeTooltipHtmlText(value)}</span>
  </div>
`;

export const buildTimeSeriesTooltipFormatter = ({
  digits = 1,
  unit = '%',
  fallbackColor = '#5B8FF9',
} = {}) => (params) => {
  if (!Array.isArray(params) || params.length === 0) return '';

  const title = params[0]?.axisValueLabel ?? params[0]?.name ?? '';
  let result = `<div style="margin-bottom:5px;">${escapeTooltipHtmlText(
    title,
  )}</div>`;
  for (const item of params) {
    const raw = Array.isArray(item?.value)
      ? item.value[item.value.length - 1]
      : item?.value;
    result += tooltipRow(
      item?.color || fallbackColor,
      item?.seriesName || '-',
      formatRangeTooltipValue(raw, { digits, unit, separator: ' ' }),
    );
  }
  return result;
};
