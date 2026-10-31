import { addCollection } from '@iconify/vue/offline';
import icons from 'virtual:icons';
import { installStandalone } from '#base/components/ui/standalone';
import App from './App.vue';
import './style.css';

addCollection(icons);
const app = createApp(App);
installStandalone(app);
app.mount('#app');
