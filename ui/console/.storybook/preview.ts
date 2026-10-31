import type { Preview } from '@storybook/vue3-vite';
import type { App } from 'vue';
import circleFlags from '@iconify-json/circle-flags/icons.json';
import lucide from '@iconify-json/lucide/icons.json';
import { addCollection, Icon as IconifyIcon } from '@iconify/vue';
import { setup } from '@storybook/vue3';
import { defineComponent } from 'vue';
import '#base/assets/css/main.css';

addCollection(lucide);
addCollection(circleFlags);

const StoryIcon = defineComponent({
  name: 'Icon',
  props: { name: { type: String, required: true }, size: { type: [String, Number], default: 16 } },
  components: { IconifyIcon },
  template: '<IconifyIcon :icon="name" :width="size" :height="size" />',
});

setup((app: App) => {
  app.component('Icon', StoryIcon);
});

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
  },
};

export default preview;
