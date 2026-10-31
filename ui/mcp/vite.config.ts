import { fileURLToPath } from 'node:url';
import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import AutoImport from 'unplugin-auto-import/vite';
import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';
import { baseAliases } from '../base/aliases';
import { iconSubset } from './iconSubset';

const here = (path: string) => fileURLToPath(new URL(path, import.meta.url));

export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    AutoImport({ imports: ['vue'], dts: 'src/auto-imports.d.ts' }),
    iconSubset([here('./src'), here('../base/app/components/ui')]),
    viteSingleFile(),
  ],
  resolve: { alias: baseAliases },
  build: { outDir: 'dist', emptyOutDir: true },
});
