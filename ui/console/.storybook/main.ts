import type { StorybookConfig } from '@storybook/vue3-vite';
import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import { baseAliases } from '../../base/aliases.ts';

const config: StorybookConfig = {
  stories: ['../app/**/*.stories.@(ts|js)', '../../base/app/**/*.stories.@(ts|js)'],
  addons: ['@storybook/addon-docs', '@storybook/addon-a11y'],
  framework: { name: '@storybook/vue3-vite', options: {} },
  async viteFinal(viteConfig) {
    return {
      ...viteConfig,
      plugins: [...(viteConfig.plugins ?? []), vue(), tailwindcss()],
      resolve: {
        ...viteConfig.resolve,
        alias: {
          ...(viteConfig.resolve?.alias as Record<string, string> | undefined),
          ...baseAliases,
        },
      },
    };
  },
};

export default config;
