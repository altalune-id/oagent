export default defineNuxtConfig({
  extends: ['@oagent/base'],
  compatibilityDate: '2026-10-06',
  components: { dirs: [] },
  modules: ['@vueuse/nuxt'],
  i18n: {
    locales: [
      { code: 'en-US', file: 'en-US.json' },
      { code: 'id-ID', file: 'id-ID.json' },
    ],
  },
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
});
