import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import { baseAliases } from './aliases.ts';

const here = (path: string) => fileURLToPath(new URL(path, import.meta.url));

export default defineNuxtConfig({
  modules: ['@nuxt/eslint', '@nuxt/icon', '@nuxtjs/i18n', '@pinia/nuxt'],
  eslint: { config: { standalone: false } },
  alias: baseAliases,
  components: { dirs: [] },
  imports: { scan: false },
  css: [here('./app/assets/css/main.css')],
  vite: { plugins: [tailwindcss() as any] },
  icon: {
    mode: 'svg',
    clientBundle: { scan: true, icons: ['circle-flags:us', 'circle-flags:id'] },
    serverBundle: false,
  },
  i18n: {
    strategy: 'no_prefix',
    defaultLocale: 'en-US',
    langDir: 'locales',
    locales: [
      { code: 'en-US', name: 'English (US)', file: 'en-US.json' },
      { code: 'id-ID', name: 'Bahasa Indonesia', file: 'id-ID.json' },
    ],
    detectBrowserLanguage: { useCookie: true, cookieKey: 'oagent_lang', redirectOn: 'root', fallbackLocale: 'en-US' },
  },
  hooks: {
    'prerender:routes': function ({ routes }) {
      routes.clear();
    },
  },
});
