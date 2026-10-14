// Writes .br and .gz next to every text asset over 1 KiB, when smaller; usage: node compress.mts <file|dir>...
import type { Buffer } from 'node:buffer';
import { readdir, readFile, rm, stat, writeFile } from 'node:fs/promises';
import { extname, join } from 'node:path';
import process from 'node:process';
import { brotliCompressSync, constants, gzipSync } from 'node:zlib';

const TEXT = new Set(['.js', '.css', '.html', '.json', '.svg', '.txt']);
const MIN_SIZE = 1024;

async function files(target: string): Promise<string[]> {
  const info = await stat(target);
  if (!info.isDirectory()) {
    return [target];
  }
  const entries = await readdir(target, { recursive: true, withFileTypes: true });
  return entries.filter(e => e.isFile()).map(e => join(e.parentPath, e.name));
}

function encode(data: Buffer): Record<string, Buffer> {
  return {
    br: brotliCompressSync(data, {
      params: {
        [constants.BROTLI_PARAM_MODE]: constants.BROTLI_MODE_TEXT,
        [constants.BROTLI_PARAM_QUALITY]: constants.BROTLI_MAX_QUALITY,
        [constants.BROTLI_PARAM_SIZE_HINT]: data.length,
      },
    }),
    gz: gzipSync(data, { level: constants.Z_BEST_COMPRESSION }),
  };
}

async function compress(file: string): Promise<number> {
  if (!TEXT.has(extname(file))) {
    return 0;
  }
  const data = await readFile(file);
  const out: Record<string, Buffer> = data.length > MIN_SIZE ? encode(data) : {};
  let written = 0;
  for (const ext of ['br', 'gz']) {
    const sibling = `${file}.${ext}`;
    if (out[ext] && out[ext].length < data.length) {
      await writeFile(sibling, out[ext]);
      written++;
    } else {
      await rm(sibling, { force: true });
    }
  }
  return written;
}

const targets = process.argv.slice(2);
if (targets.length === 0) {
  console.error('usage: node compress.mts <file|dir>...');
  process.exit(2);
}
let total = 0;
for (const target of targets) {
  for (const file of await files(target)) {
    total += await compress(file);
  }
}
console.warn(`compress: wrote ${total} files under ${targets.join(', ')}`);
