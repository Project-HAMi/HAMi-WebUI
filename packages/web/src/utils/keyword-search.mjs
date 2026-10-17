const lower = (text) => String(text ?? '').toLowerCase();
// Fields may also match without "-" and "_", so "a100sxm4" finds "NVIDIA-A100-SXM4-80GB";
// separators typed in a keyword still count, so "node-1" does not match "node1".
const variants = (text) => {
  const plain = lower(text);
  return [plain, plain.replace(/[-_]/g, '')];
};

export const parseKeywords = (input) => lower(input).trim().split(/\s+/).filter(Boolean);

// Every keyword must appear within a single field; fields are not joined.
export const matchesKeywords = (keywords, fields) => {
  const texts = fields.flatMap(variants);
  return keywords.every((keyword) => texts.some((text) => text.includes(keyword)));
};

export const createOptionFilter = (fieldsOf = () => []) => (input, option) =>
  matchesKeywords(parseKeywords(input), [option.label, ...fieldsOf(option.value)]);

// Long lists render only the first matches; the rest stay reachable by typing.
export const limitOptions = (options, input, fieldsOf = () => [], limit = 100) => {
  const keywords = parseKeywords(input);
  const matched = keywords.length
    ? options.filter((option) => matchesKeywords(keywords, [option.label, ...fieldsOf(option.value)]))
    : options;
  return { options: matched.slice(0, limit), hidden: Math.max(matched.length - limit, 0) };
};
