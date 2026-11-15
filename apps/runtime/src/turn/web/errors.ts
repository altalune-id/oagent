export const ERROR_CLASSES = ['call_limit', 'over_budget', 'timeout', 'blocked_host', 'http_status', 'unsupported_type', 'invalid_request', 'bad_key', 'quota', 'rate_limited', 'upstream_error'] as const;

export type ErrorClass = typeof ERROR_CLASSES[number];

export class WebToolError extends Error {
  override name = 'WebToolError';
  readonly class: ErrorClass;
  readonly host: string | undefined;
  readonly status: number | undefined;
  readonly billed: boolean;

  constructor(cls: ErrorClass, host?: string, status?: number, options: { billed?: boolean } = {}) {
    super([cls, host === undefined ? '' : ` (host ${host})`, status === undefined ? '' : ` (status ${status})`].join(''));
    this.class = cls;
    this.host = host;
    this.status = status;
    this.billed = options.billed ?? false;
  }
}
