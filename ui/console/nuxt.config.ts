import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';

export default defineNuxtConfig({
  compatibilityDate: '2026-10-06',
  modules: ['@nuxt/eslint', '@nuxt/icon', '@pinia/nuxt', '@vueuse/nuxt', '@nuxtjs/i18n'],
  eslint: { config: { standalone: false } },
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
    detectBrowserLanguage: {
      useCookie: true,
      cookieKey: 'oagent_lang',
      redirectOn: 'root',
      fallbackLocale: 'en-US',
    },
  },
  alias: {
    '@shared/scope': fileURLToPath(new URL('../../scope', import.meta.url)),
    '@shared/apperror': fileURLToPath(new URL('../../apperror', import.meta.url)),
  },
  components: { dirs: [] },
  imports: { scan: false },
  css: ['~/assets/css/main.css'],
  vite: { plugins: [tailwindcss() as any] },
  app: {
    head: {
      title: 'oagent',
      htmlAttrs: { lang: 'en' },
      link: [{ key: 'icon', rel: 'icon', href: 'data:,' }],
    },
  },
  runtimeConfig: {
    public: { apiUrl: '', appVersion: '' },
  },
  $development: {
    ssr: false,
    devtools: {
      enabled: true,
      vscode: {},
    },
    devServer: { port: 8280, host: '0.0.0.0' },
    nitro: {
      devProxy: { '/api': { target: 'http://localhost:5150/api', changeOrigin: true } },
    },
  },
  $production: {
    ssr: false,
  },
  hooks: {
    'prerender:routes': function ({ routes }) {
      routes.clear();
    },
  },
});
