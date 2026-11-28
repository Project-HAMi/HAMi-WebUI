import { createApp } from 'vue';
import App from './App.vue';
import router from './router';
import { installChunkRecovery } from './router/chunk-recovery.mjs';
import installTDesign from './plugins/tdesign';
import './plugins/echarts';
import 'normalize.css/normalize.css';
import '@/styles/index.scss';

import installIcons from '@/icons';
import i18n from '@/locales';

const app = createApp(App);
app.use(i18n);
installTDesign(app);
installIcons(app);

installChunkRecovery(router);
app.use(router).mount('#app');
