import { t } from '../copy';

export interface ToolResult {
  isError?: boolean;
  content?: { type: string, text?: string }[];
  structuredContent?: Record<string, unknown>;
}

// ToolCaller is the part of the MCP App bridge the views use.
export interface ToolCaller {
  callServerTool: (params: { name: string, arguments: Record<string, unknown> }) => Promise<ToolResult>;
}

export interface ErrorEnvelope {
  code: string;
  message: string;
  meta: Record<string, unknown>;
}

// errorEnvelope reads OAgent's JSON error from a failed tool's text, or null for plain text.
export function errorEnvelope(text: string): ErrorEnvelope | null {
  try {
    const parsed = JSON.parse(text) as Partial<ErrorEnvelope> | null;
    if (!parsed || typeof parsed.code !== 'string') {
      return null;
    }
    return { code: parsed.code, message: String(parsed.message ?? ''), meta: parsed.meta ?? {} };
  } catch {
    return null;
  }
}

// errorText is what a failed tool result means, in the console's words with its code: never the raw JSON.
export function errorText(res: ToolResult): string {
  const text = (res.content ?? []).map(c => c.text ?? '').filter(Boolean).join(' ');
  if (!text) {
    return t('mcp.tool_failed');
  }
  const envelope = errorEnvelope(text);
  if (!envelope) {
    return text;
  }
  const reason = typeof envelope.meta.reason === 'string' ? envelope.meta.reason : '';
  for (const key of [reason && `apperror.${envelope.code}_${reason}`, `apperror.${envelope.code}`].filter(Boolean)) {
    const copy = t(key, envelope.meta);
    if (copy !== key && !/\{\w+\}/.test(copy)) {
      return `${copy} (${envelope.code})`;
    }
  }
  return `${envelope.message || t('mcp.tool_failed')} (${envelope.code})`;
}

// callTool runs a tool from the view and returns its structuredContent; a refusal throws its text.
export async function callTool(app: ToolCaller, name: string, args: Record<string, unknown>): Promise<Record<string, unknown>> {
  const res = await app.callServerTool({ name, arguments: args });
  if (res.isError) {
    throw new Error(errorText(res));
  }
  return res.structuredContent ?? {};
}
