import { createI18n } from 'vue-i18n';
import Cookies from 'js-cookie';
import enLocale from './en';
import zhLocale from './zh';
import { resolveLanguage } from './language.mjs';

export const LANG_KEY = 'language';

export const getLanguage = () => {
  const browserLanguage =
    typeof navigator === 'undefined'
      ? undefined
      : navigator.language || navigator.browserLanguage;

  return resolveLanguage(Cookies.get(LANG_KEY), browserLanguage);
};

const i18n = createI18n({
  legacy: false,
  locale: getLanguage(),
  fallbackLocale: 'en',
  globalInjection: true,
  messages: {
    en: enLocale,
    zh: zhLocale,
  },
});

export default i18n;
