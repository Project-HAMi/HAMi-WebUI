export const SUPPORTED_LANGUAGES = Object.freeze(['en', 'zh']);

const normalizeLanguage = (language) =>
  typeof language === 'string' ? language.trim().toLowerCase() : '';

export function resolveLanguage(preferredLanguage, browserLanguage) {
  const preferred = normalizeLanguage(preferredLanguage);
  if (SUPPORTED_LANGUAGES.includes(preferred)) return preferred;

  const browser = normalizeLanguage(browserLanguage).replaceAll('_', '-');
  return browser === 'zh' || browser.startsWith('zh-') ? 'zh' : 'en';
}

export function toDocumentLanguage(language) {
  return language === 'zh' ? 'zh-CN' : 'en';
}
