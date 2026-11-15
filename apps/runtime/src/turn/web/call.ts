import type { ToolContext } from '@strands-agents/sdk';
import type { WebToolName } from '../../interventions/index.ts';
import type { Logger } from '../../log.ts';
import type { Budget, TurnUsage } from '../limits.ts';
import { redact, SsrfRefusedError } from '../../egress/index.ts';
import { attr, escapeToolOutput } from '../delimit.ts';
import { WebToolError } from './errors.ts';

export { ERROR_CLASSES } from './errors.ts';

export class WebCalls {
  readonly max: number;
  searchesInFlight = 0;
  #taken = 0;

  constructor(max: number) {
    this.max = max;
  }

  take(): void {
    if (this.#taken >= this.max) {
      throw new WebToolError('call_limit');
    }
    this.#taken++;
  }
}

export type WebTurn = {
  run: string,
  log: Logger,
  calls: WebCalls,
  secrets: readonly string[],
  timeoutMs: number,
  usage: TurnUsage,
  budget: Budget | null,
};

export type CallResult = {
  text: string,
  attrs?: Record<string, string>,
  host?: string,
  status?: number,
  bytes?: number,
};

export function wrapOutput(tool: WebToolName, text: string, attrs: Record<string, string> = {}): string {
  const extra = Object.entries(attrs).map(([k, v]) => ` ${k}="${attr(v)}"`).join('');
  return `<tool_output tool="${tool}"${extra}>\n${escapeToolOutput(text)}\n</tool_output>`;
}

function failureOf(error: unknown): WebToolError {
  for (let e: unknown = error, depth = 0; e instanceof Error && depth < 5; e = e.cause, depth++) {
    if (e instanceof WebToolError) {
      return e;
    }
    if (e instanceof SsrfRefusedError) {
      return new WebToolError('blocked_host', e.host);
    }
    if (e.message.startsWith('egress: ')) {
      return new WebToolError('blocked_host');
    }
    if (e.name === 'TimeoutError' || e.name === 'AbortError') {
      return new WebToolError('timeout');
    }
  }
  return new WebToolError('upstream_error');
}

export async function runCall(turn: WebTurn, tool: WebToolName, ctx: ToolContext | undefined, timeoutMs: number, call: (signal: AbortSignal) => Promise<CallResult>): Promise<string> {
  const started = Date.now();
  try {
    turn.calls.take();
    const signal = AbortSignal.any([...(ctx?.cancelSignal ? [ctx.cancelSignal] : []), AbortSignal.timeout(timeoutMs)]);
    const out = await call(signal);
    turn.log.info({ run: turn.run, tool, host: out.host, status: out.status, bytes: out.bytes, ms: Date.now() - started }, 'web call');
    const secrets = [...turn.secrets];
    const attrs = Object.fromEntries(Object.entries(out.attrs ?? {}).map(([k, v]) => [k, redact(v, secrets)]));
    return wrapOutput(tool, redact(out.text, secrets), attrs);
  } catch (error) {
    const failure = failureOf(error);
    turn.log.warn({ run: turn.run, tool, class: failure.class, host: failure.host, status: failure.status, ms: Date.now() - started, ...(failure !== error && { error }) }, 'web call failed');
    throw new Error(`${tool} failed: ${failure.message}`);
  }
}
