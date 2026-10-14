import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    lib: {
      entry: 'src/password/index.ts',
      formats: ['es'],
      fileName: () => 'password.js',
    },
    outDir: '../../internal/web/static',
    emptyOutDir: false,
    minify: true,
  },
});
