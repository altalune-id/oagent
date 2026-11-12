import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { settable, UploadFailedError, xhrPut } from './upload';

type Listener = (e: { lengthComputable: boolean, loaded: number, total: number }) => void;

class FakeXHR {
  static last: FakeXHR;
  method = '';
  url = '';
  body: unknown;
  status = 0;
  withCredentials = true;
  headers: Record<string, string> = {};
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  upload = { onprogress: null as Listener | null };

  constructor() {
    FakeXHR.last = this;
  }

  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }

  setRequestHeader(name: string, value: string) {
    this.headers[name] = value;
  }

  send(body: unknown) {
    this.body = body;
  }

  abort() {
    this.onabort?.();
  }
}

const file = new File(['%PDF-1.7 hello'], 'refund-policy.pdf', { type: 'application/pdf' });

describe('xhrPut', () => {
  beforeEach(() => {
    vi.stubGlobal('XMLHttpRequest', FakeXHR);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends the file itself by PUT to the presigned URL, without cookies, with the signed headers', async () => {
    const done = xhrPut('https://bucket.example.com/k/doc1?X-Amz-Signature=abc', file, { 'Content-Type': 'application/pdf', 'x-amz-checksum-sha256': 'c2hh' }, () => {});
    const xhr = FakeXHR.last;
    expect(xhr.method).toBe('PUT');
    expect(xhr.url).toBe('https://bucket.example.com/k/doc1?X-Amz-Signature=abc');
    expect(xhr.body).toBe(file);
    expect(xhr.withCredentials).toBe(false);
    expect(xhr.headers).toEqual({ 'Content-Type': 'application/pdf', 'x-amz-checksum-sha256': 'c2hh' });
    xhr.status = 200;
    xhr.onload!();
    await expect(done).resolves.toBeUndefined();
  });

  it('skips the headers a browser refuses to let a script set', async () => {
    const done = xhrPut('https://b/k', file, { 'Content-Length': '14', 'Host': 'b', 'Sec-Fetch-Mode': 'cors', 'Proxy-Authorization': 'x', 'Content-Type': 'application/pdf' }, () => {});
    expect(FakeXHR.last.headers).toEqual({ 'Content-Type': 'application/pdf' });
    FakeXHR.last.status = 204;
    FakeXHR.last.onload!();
    await done;
  });

  it('reports progress as a fraction, and ignores events without a known length', async () => {
    const seen: number[] = [];
    const done = xhrPut('https://b/k', file, {}, f => seen.push(f));
    const progress = FakeXHR.last.upload.onprogress!;
    progress({ lengthComputable: false, loaded: 3, total: 0 });
    progress({ lengthComputable: true, loaded: 7, total: 14 });
    progress({ lengthComputable: true, loaded: 14, total: 14 });
    FakeXHR.last.status = 200;
    FakeXHR.last.onload!();
    await done;
    expect(seen).toEqual([0.5, 1]);
  });

  it('fails with the HTTP status when storage refuses the PUT', async () => {
    const done = xhrPut('https://b/k', file, {}, () => {});
    FakeXHR.last.status = 403;
    FakeXHR.last.onload!();
    await expect(done).rejects.toEqual(new UploadFailedError(403));
  });

  it('refuses a plain-http upload URL before any request, except local dev storage', async () => {
    const before = new FakeXHR();
    await expect(xhrPut('http://bucket.example.com/k', file, {}, () => {})).rejects.toMatchObject({ status: 0 });
    await expect(xhrPut('javascript:alert(1)', file, {}, () => {})).rejects.toMatchObject({ status: 0 });
    await expect(xhrPut('not a url', file, {}, () => {})).rejects.toMatchObject({ status: 0 });
    expect(FakeXHR.last).toBe(before);
    for (const url of ['http://localhost:9000/b/k', 'http://127.0.0.1:9000/b/k']) {
      const done = xhrPut(url, file, {}, () => {});
      expect(FakeXHR.last.url).toBe(url);
      FakeXHR.last.status = 200;
      FakeXHR.last.onload!();
      await done;
    }
  });

  it('fails with status 0 on a network error or an abort', async () => {
    const lost = xhrPut('https://b/k', file, {}, () => {});
    FakeXHR.last.onerror!();
    await expect(lost).rejects.toMatchObject({ status: 0 });
    const aborted = xhrPut('https://b/k', file, {}, () => {});
    FakeXHR.last.abort();
    await expect(aborted).rejects.toMatchObject({ status: 0 });
  });
});

describe('settable', () => {
  it('refuses forbidden request headers and the Proxy- and Sec- prefixes, in any case', () => {
    for (const name of ['Cookie', 'transfer-encoding', 'Origin', 'content-length', 'HOST', 'Connection', 'proxy-x', 'sec-ch-ua']) {
      expect(settable(name), name).toBe(false);
    }
    for (const name of ['Content-Type', 'Content-MD5', 'x-amz-checksum-sha256', 'Cache-Control']) {
      expect(settable(name), name).toBe(true);
    }
  });
});
