import { createStore } from 'vuex';
import createPersistedState from 'vuex-persistedstate';
import getters from './getters';

// https://webpack.js.org/guides/dependency-management/#requirecontext
const modulesFiles = require.context('./modules', true, /\.js$/);

// you do not need `import app from './modules/app'`
// it will auto require all vuex module from modules file
const modules = modulesFiles.keys().reduce((modules, modulePath) => {
  // set './app.js' => 'app'
  const moduleName = modulePath.replace(/^\.\/(.*)\.\w+$/, '$1');
  const value = modulesFiles(modulePath);
  modules[moduleName] = value.default;
  return modules;
}, {});

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
