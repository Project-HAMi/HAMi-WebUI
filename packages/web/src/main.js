import { createApp } from 'vue';
import App from './App.vue';
import router from './router';
import store from './store';
import installElementPlus from './plugins/element';
import 'normalize.css/normalize.css'; // a modern alternative to CSS resets
import '@/styles/index.scss'; // global css

// 导入 svgIcon
import installIcons from '@/icons';
import Message from '@/components/Message/index.js';
import Confirm from '@/components/Confirm/index.js';
import components from '@/components';
import '@/styles/element/index.scss';

const app = createApp(App);
installElementPlus(app);
installIcons(app);

app.use(components);

app.use(store).use(router).use(Confirm).use(Message).mount('#app');
