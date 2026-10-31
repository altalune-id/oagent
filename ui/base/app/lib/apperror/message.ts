import { ConnectError } from '@connectrpc/connect';

import { ErrorDetailSchema } from '#gen/apperror/v1/error_pb';

export type Translate = (key: string, params?: Record<string, unknown>) => string;
export type HasKey = (key: string) => boolean;

export interface ErrorView {
  message: string;
  code?: string;
  requestId?: string;
}

export interface ErrorLabels {
  scope?: (name: string) => string;
  time?: (iso: string) => string;
}

// errorCode is the app error code an RPC failed with, or '' when there is none.
export function errorCode(err: unknown): string {
  return err instanceof ConnectError ? (err.findDetails(ErrorDetailSchema)[0]?.code ?? '') : '';
}

// errorMeta is the params an RPC failure carries, or none.
export function errorMeta(err: unknown): Record<string, string> {
  return err instanceof ConnectError ? { ...err.findDetails(ErrorDetailSchema)[0]?.meta } : {};
}

// Operator diagnostics: never shown, never passed to a message.
const hidden = new Set(['reason', 'trace_id', 'request_id', 'underlying_error']);

// Pure (no Nuxt imports) so it runs under the node Vitest preset.
export function describeError(err: unknown, t: Translate, te: HasKey, fallback: string, labels: ErrorLabels = {}): ErrorView {
  if (!(err instanceof ConnectError)) {
    return { message: fallback };
  }
  const detail = err.findDetails(ErrorDetailSchema)[0];
  if (!detail) {
    return { message: fallback };
  }
  const view: ErrorView = { message: '', code: detail.code };
  if (detail.requestId) {
    view.requestId = detail.requestId;
  }

  const params: Record<string, string> = {};
  for (const [key, value] of Object.entries(detail.meta)) {
    if (!hidden.has(key)) {
      params[key] = label(key, value, te, t, labels);
    }
  }

  const reason = detail.meta.reason;
  const keys = reason ? [`apperror.${detail.code}_${reason}`, `apperror.${detail.code}`] : [`apperror.${detail.code}`];
  const key = keys.find(k => te(k));
  view.message = key ? t(key, params) : err.rawMessage || fallback;
  return view;
}

function label(key: string, value: string, te: HasKey, t: Translate, labels: ErrorLabels): string {
  if ((key === 'scope' || key === 'permission') && labels.scope) {
    return labels.scope(value);
  }
  if (key === 'time' && labels.time && !Number.isNaN(Date.parse(value))) {
    return labels.time(value);
  }
  if (key === 'field' && te(`field.${value}`)) {
    return t(`field.${value}`);
  }
  const eventKey = `webhooks.event_${value.replaceAll('.', '_')}`;
  if (key === 'event' && te(eventKey)) {
    return t(eventKey);
  }
  return value;
}

// ShownError is what the console renders: the message, plus a muted support reference when the server sent a request id.
export interface ShownError {
  message: string;
  reference?: string;
}

export function toShown(view: ErrorView, ref: (id: string) => string): ShownError {
  return view.requestId ? { message: view.message, reference: ref(view.requestId) } : { message: view.message };
}
