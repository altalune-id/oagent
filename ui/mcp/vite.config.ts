import vue from '@vitejs/plugin-vue';
import AutoImport from 'unplugin-auto-import/vite';
import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';

export default defineConfig({
  plugins: [
    vue(),
    AutoImport({ imports: ['vue'], dts: 'src/auto-imports.d.ts' }),
    viteSingleFile(),
  ],
  build: { outDir: 'dist', emptyOutDir: true },
});
