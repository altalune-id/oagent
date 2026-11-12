export type UploadState = 'queued' | 'uploading' | 'done' | 'failed';

export interface UploadItem {
  key: string;
  name: string;
  size: string;
  state: UploadState;
  progress: number;
  error?: string;
}

export type UploadTransport = (url: string, file: Blob, headers: Record<string, string>, progress: (fraction: number) => void) => Promise<void>;

export interface UploadReject { file: File; reason: 'type' | 'size'; }

// acceptsFile reports whether a file matches an input accept list (extensions, exact or wildcard MIME types).
export function acceptsFile(file: File, accept: string): boolean {
  const tokens = accept.split(',').map(t => t.trim().toLowerCase()).filter(Boolean);
  const name = file.name.toLowerCase();
  const type = file.type.toLowerCase();
  return !tokens.length || tokens.some(t => t.startsWith('.') ? name.endsWith(t) : t.endsWith('/*') ? type.startsWith(t.slice(0, -1)) : type === t);
}

// UploadFailedError is a refused or lost upload; status 0 means the request never got an answer.
export class UploadFailedError extends Error {
  constructor(readonly status: number) {
    super(`upload failed: ${status}`);
    this.name = 'UploadFailedError';
  }
}

// https://fetch.spec.whatwg.org/#forbidden-request-header
const forbidden = new Set(['accept-charset', 'accept-encoding', 'access-control-request-headers', 'access-control-request-method', 'connection', 'content-length', 'cookie', 'cookie2', 'date', 'dnt', 'expect', 'host', 'keep-alive', 'origin', 'referer', 'set-cookie', 'te', 'trailer', 'transfer-encoding', 'upgrade', 'via']);

// settable reports whether a script may set the request header name.
export function settable(name: string): boolean {
  const n = name.toLowerCase();
  return !forbidden.has(n) && !n.startsWith('proxy-') && !n.startsWith('sec-');
}

// uploadable reports whether files may go to a URL: https, or plain http to local dev storage only.
export function uploadable(url: string): boolean {
  try {
    const u = new URL(url);
    return u.protocol === 'https:' || (u.protocol === 'http:' && (u.hostname === 'localhost' || u.hostname === '127.0.0.1'));
  } catch {
    return false;
  }
}

// xhrPut PUTs the file to a presigned URL with XMLHttpRequest and reports upload progress.
export const xhrPut: UploadTransport = (url, file, headers, progress) => new Promise((resolve, reject) => {
  if (!uploadable(url)) {
    reject(new UploadFailedError(0));
    return;
  }
  const xhr = new XMLHttpRequest();
  xhr.open('PUT', url);
  xhr.withCredentials = false;
  for (const [name, value] of Object.entries(headers)) {
    if (settable(name)) {
      xhr.setRequestHeader(name, value);
    }
  }
  xhr.upload.onprogress = (e) => {
    if (e.lengthComputable) {
      progress(e.loaded / e.total);
    }
  };
  xhr.onload = () => {
    if (xhr.status >= 200 && xhr.status < 300) {
      resolve();
    } else {
      reject(new UploadFailedError(xhr.status));
    }
  };
  xhr.onerror = () => reject(new UploadFailedError(0));
  xhr.onabort = () => reject(new UploadFailedError(0));
  xhr.send(file);
});
