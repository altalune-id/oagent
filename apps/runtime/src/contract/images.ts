import type { TurnRequest } from './request.ts';
import { Buffer } from 'node:buffer';

import { TurnRefusedError } from './errors.ts';
import { CONTRACT_VERSION } from './request.ts';

export const MAX_IMAGES = 3;
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;

export type ImageMime = 'image/jpeg' | 'image/png' | 'image/webp';

const MAGIC: Record<ImageMime, (b: Buffer) => boolean> = {
  'image/jpeg': b => b.length >= 3 && b[0] === 0xFF && b[1] === 0xD8 && b[2] === 0xFF,
  'image/png': b => b.length >= 8 && b.subarray(0, 8).equals(Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])),
  'image/webp': b => b.length >= 12 && b.toString('latin1', 0, 4) === 'RIFF' && b.toString('latin1', 8, 12) === 'WEBP',
};

const BASE64 = /^[A-Z0-9+/]*={0,2}$/i;

function isBase64(value: string): boolean {
  return value.length % 4 === 0 && BASE64.test(value);
}

export type InputImage = {
  mime: ImageMime,
  bytes: Buffer,
};

export type InputPart = { text: string } | { image: InputImage };

function decodeImage(part: unknown): InputImage {
  const p = part as { type?: string, source?: { type?: string, value?: string, mimeType?: string } };
  const mime = p.source?.mimeType as ImageMime | undefined;
  if (p.type !== 'image' || p.source?.type !== 'data' || mime === undefined || !Object.hasOwn(MAGIC, mime) || !isBase64(p.source.value ?? '')) {
    throw new TurnRefusedError(422, 'image_invalid');
  }
  if (Buffer.byteLength(p.source.value!, 'base64') > MAX_IMAGE_BYTES) {
    throw new TurnRefusedError(422, 'image_too_large');
  }
  const bytes = Buffer.from(p.source.value!, 'base64');
  if (!MAGIC[mime](bytes)) {
    throw new TurnRefusedError(422, 'image_invalid');
  }
  return { mime, bytes };
}

export function inputParts(request: TurnRequest): InputPart[] {
  if (request.input.kind !== 'message') {
    return [];
  }
  const content: unknown = request.input.message.content;
  if (typeof content === 'string') {
    return [{ text: content }];
  }
  const parts = content as { type: string, text?: string }[];
  const images = parts.filter(p => p.type !== 'text');
  if (images.length > 0 && request.contractVersion !== CONTRACT_VERSION) {
    throw new TurnRefusedError(422, 'image_invalid');
  }
  if (images.length > MAX_IMAGES) {
    throw new TurnRefusedError(422, 'too_many_images');
  }
  return parts.map(p => (p.type === 'text' ? { text: p.text ?? '' } : { image: decodeImage(p) }));
}

export function checkImages(request: TurnRequest): void {
  inputParts(request);
}
