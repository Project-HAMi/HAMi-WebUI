export const escapeTooltipHtmlText = (value) =>
  String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');

export const formatCardTypeTooltip = ({ name, value }, unit) => {
  const suffix = unit ? ` ${escapeTooltipHtmlText(unit)}` : '';

  return `${escapeTooltipHtmlText(name)}: ${escapeTooltipHtmlText(
    value,
  )}${suffix}`;
};
