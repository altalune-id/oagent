import icons from 'virtual:icons';
import { addIcons, installStandalone } from '#base/components/ui/standalone';
import App from './App.vue';
import './style.css';

addIcons(icons);
const app = createApp(App);
installStandalone(app);
app.mount('#app');
