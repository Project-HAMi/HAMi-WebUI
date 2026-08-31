export const promQLStringLiteral = (value) => JSON.stringify(String(value));

const PROMQL_TEMPLATE_VARIABLE = /\$([a-zA-Z_][a-zA-Z0-9_]*)/g;

export const renderPromQLTemplate = (query, variables) =>
  query.replace(PROMQL_TEMPLATE_VARIABLE, (placeholder, name) => {
    if (!Object.hasOwn(variables, name)) {
      throw new TypeError(`Missing PromQL template variable: ${name}`);
    }
    return promQLStringLiteral(variables[name]);
  });
