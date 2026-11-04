import type { Preview } from '@storybook/vue3-vite';
import type { App } from 'vue';
import circleFlags from '@iconify-json/circle-flags/icons.json';
import lucide from '@iconify-json/lucide/icons.json';
import { setup } from '@storybook/vue3';
import { themeFamilies } from '#base/components/layout/useTheme';
import { addIcons, installStandalone } from '#base/components/ui/standalone';
import '#base/assets/css/main.css';

addIcons(lucide);
addIcons(circleFlags);

setup((app: App) => {
  installStandalone(app);
});

const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'Theme',
      toolbar: { title: 'Theme', icon: 'mirror', items: themeFamilies.flatMap(f => Object.values(f.themes)), dynamicTitle: true },
    },
  },
  initialGlobals: { theme: 'oagent-light' },
  loaders: [async () => {
    await Promise.all(['400 1em "Instrument Sans Variable"', '600 1em "Instrument Sans Variable"', '400 1em "JetBrains Mono Variable"']
      .map(font => document.fonts.load(font)));
    return {};
  }],
  decorators: [
    (story, context) => {
      document.documentElement.dataset.theme = context.globals.theme as string;
      return { components: { story }, template: '<div class="bg-base-100 p-4 text-sm leading-[normal] text-base-content"><story /></div>' };
    },
  ],
  parameters: {
    backgrounds: { disable: true },
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
  },
};

export default preview;
