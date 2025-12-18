import { createI18n } from 'vue-i18n';
import Cookies from 'js-cookie';
import enLocale from './en';
import zhLocale from './zh';

// 定义语言 Key
export const LANG_KEY = 'language';

// 获取当前语言
export const getLanguage = () => {
  const chooseLanguage = Cookies.get(LANG_KEY);
  if (chooseLanguage) return chooseLanguage;

  // 如果没有选择过，自动检测浏览器语言
  const language = (navigator.language || navigator.browserLanguage).toLowerCase();
  const locales = [enLocale, zhLocale];
  for (const locale of locales) {
    if (language.indexOf(locale) > -1) {
      return locale;
    }
  }
  return 'zh'; // 默认中文
};

const i18n = createI18n({
  legacy: false, // 使用 Composition API
  locale: getLanguage(),
  fallbackLocale: 'en',
  globalInjection: true, // 全局注入 $t
  messages: {
    en: enLocale,
    zh: zhLocale,
  },
});

export default i18n;
