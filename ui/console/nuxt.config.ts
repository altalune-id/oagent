import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  compatibilityDate: '2026-10-06',
  devtools: { enabled: true },
  modules: ['@pinia/nuxt', '@vueuse/nuxt'],
  pinia: { storesDirs: ['./app/stores/**'] },
  components: {
    dirs: [
      { path: '~/components', pathPrefix: true },
      { path: '~/components/primitives', pathPrefix: false },
    ],
  },
  imports: { scan: false },
  css: ['~/assets/css/main.css'],
  vite: { plugins: [tailwindcss() as any] },
  app: {
    head: {
      title: 'oagent',
      htmlAttrs: { lang: 'en' },
      link: [{ rel: 'icon', href: 'data:,' }],
    },
  },
  runtimeConfig: {
    public: { apiUrl: '' },
  },
  $development: {
    ssr: false,
    devServer: { port: 8280, host: '0.0.0.0' },
    nitro: {
      devProxy: { '/api': { target: 'http://localhost:5150/api', changeOrigin: true } },
    },
  },
  $production: {
    ssr: false,
  },
  hooks: {
    'prerender:routes'({ routes }) {
      routes.clear()
    },
  },
})
