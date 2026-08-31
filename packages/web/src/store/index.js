import { createStore } from 'vuex';
import createPersistedState from 'vuex-persistedstate';
import getters from './getters';

const modules = {};
const viteModules = import.meta.glob('./modules/*.js', { eager: true });

Object.keys(viteModules).forEach((key) => {
  const moduleName = key.replace(/^\.\/modules\/(.*)\.\w+$/, '$1');
  modules[moduleName] = viteModules[key].default;
});

const global = createPersistedState({
  key: 'global',
  storage: window.localStorage,
  paths: ['global'],
});

const store = new createStore({
  state: {},
  modules,
  getters,
  plugins: [global],
});

export default store;
