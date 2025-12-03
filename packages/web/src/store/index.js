import { createStore } from 'vuex';
import createPersistedState from 'vuex-persistedstate';
import getters from './getters';

// https://webpack.js.org/guides/dependency-management/#requirecontext
let modules = {};

// Vite support
// import.meta.glob is replaced during build time by Vite
const viteModules = import.meta.glob('./modules/*.js', { eager: true });

if (Object.keys(viteModules).length > 0) {
  Object.keys(viteModules).forEach((key) => {
    const moduleName = key.replace(/^\.\/modules\/(.*)\.\w+$/, '$1');
    modules[moduleName] = viteModules[key].default;
  });
} else {
  // Webpack support
  if (typeof require.context !== 'undefined') {
    const modulesFiles = require.context('./modules', true, /\.js$/);
    modules = modulesFiles.keys().reduce((modules, modulePath) => {
      const moduleName = modulePath.replace(/^\.\/(.*)\.\w+$/, '$1');
      const value = modulesFiles(modulePath);
      modules[moduleName] = value.default;
      return modules;
    }, {});
  }
}

const global = createPersistedState({
  key: 'global',
  storage: window.localStorage, //window.localStorage
  paths: ['global'],
});

const store = new createStore({
  state: {},
  modules,
  getters,
  // 将各个插件引用
  plugins: [global],
});

export default store;
