import type { TurnRequest } from './index.ts';
import { Buffer } from 'node:buffer';
import { describe, expect, it } from 'vitest';

import { goldenRequest } from '../test/turn-fixtures.ts';
import { checkImages, MAX_IMAGE_BYTES, MAX_IMAGES } from './images.ts';
import { TurnRefusedError, validateRequest } from './index.ts';

const PNG = Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13]).toString('base64');
const JPEG = Buffer.from([0xFF, 0xD8, 0xFF, 0xE0, 0, 16]).toString('base64');
const WEBP = Buffer.from('RIFF\u0000\u0000\u0000\u0000WEBPVP8 ', 'latin1').toString('base64');

function image(value: string, mimeType: string): Record<string, unknown> {
  return { type: 'image', source: { type: 'data', value, mimeType } };
}

function withContent(content: unknown, version = 2): TurnRequest {
  const raw = structuredClone(goldenRequest('plain_reply')) as unknown as Record<string, unknown>;
  raw.contractVersion = version;
  (raw.input as { message: { content: unknown } }).message.content = content;
  const result = validateRequest(raw);
  if (!result.ok) {
    throw new Error(`invalid: ${result.fields.join(',')}`);
  }
  return result.request;
}

function refusal(request: TurnRequest): string | undefined {
  try {
    checkImages(request);
    return undefined;
  } catch (error) {
    expect(error).toBeInstanceOf(TurnRefusedError);
    expect((error as TurnRefusedError).status).toBe(422);
    return (error as TurnRefusedError).code;
  }
}

describe('image input', () => {
  it('accepts up to three jpeg, png or webp images with text', () => {
    expect(refusal(withContent([{ type: 'text', text: 'my receipt' }, image(PNG, 'image/png'), image(JPEG, 'image/jpeg'), image(WEBP, 'image/webp')]))).toBeUndefined();
    expect(refusal(withContent('plain text'))).toBeUndefined();
  });

  it('refuses a fourth image', () => {
    const four = Array.from({ length: MAX_IMAGES + 1 }, () => image(PNG, 'image/png'));
    expect(refusal(withContent(four))).toBe('too_many_images');
  });

  it('refuses an image over the decoded size cap', () => {
    const header = Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);
    const big = Buffer.concat([header, Buffer.alloc(MAX_IMAGE_BYTES - header.length + 1)]).toString('base64');
    expect(refusal(withContent([image(big, 'image/png')]))).toBe('image_too_large');
    const edge = Buffer.concat([header, Buffer.alloc(MAX_IMAGE_BYTES - header.length)]).toString('base64');
    expect(refusal(withContent([image(edge, 'image/png')]))).toBeUndefined();
  });

  it.each([
    ['a gif', [image(PNG, 'image/gif')]],
    ['an inherited object key as the type', [image(PNG, 'toString')]],
    ['a url source', [{ type: 'image', source: { type: 'url', value: 'https://example.com/a.png' } }]],
    ['an audio part', [{ type: 'audio', source: { type: 'data', value: PNG, mimeType: 'audio/ogg' } }]],
    ['a document part', [{ type: 'document', source: { type: 'data', value: PNG, mimeType: 'application/pdf' } }]],
    ['bad base64', [image('not base64!!', 'image/png')]],
    ['bytes that are not the declared type', [image(JPEG, 'image/png')]],
    ['an unfilled stored part', [{ ...image('', 'image/jpeg'), id: 'msg_1' }]],
  ])('refuses %s', (_name, content) => {
    expect(refusal(withContent(content))).toBe('image_invalid');
  });

  it('refuses images on contract version 1, so an old caller never has photos dropped silently', () => {
    expect(refusal(withContent([image(PNG, 'image/png')], 1))).toBe('image_invalid');
    expect(refusal(withContent('text only', 1))).toBeUndefined();
  });
});
