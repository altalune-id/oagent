import { t } from '../copy';

const perMillion = 1_000_000n;

function usd(micros: unknown): string {
  const text = String(micros ?? '0');
  if (!/^\d+$/.test(text)) {
    return text;
  }
  const m = BigInt(text);
  const frac = (m % perMillion).toString().padStart(6, '0').replace(/0+$/, '');
  return `$${m / perMillion}${frac ? `.${frac}` : ''}`;
}

// modelSummary lists a provider's models with their input and output prices in USD per 1M tokens.
export function modelSummary(value: unknown): string {
  if (!Array.isArray(value) || value.length === 0) {
    return '—';
  }
  return value.map((m: { id?: string, prices?: { inputMicros?: unknown, outputMicros?: unknown } }) => {
    const id = m.id ?? '';
    return m.prices ? `${id} (${t('mcp.model_price', { input: usd(m.prices.inputMicros), output: usd(m.prices.outputMicros) })})` : `${id} (${t('mcp.model_unpriced')})`;
  }).join(', ');
}

// timeoutText shows a timeout in milliseconds as seconds.
export function timeoutText(value: unknown): string {
  const ms = Number(value ?? 0);
  return `${ms / 1000} s`;
}

export const authKinds = {
  MCP_SERVER_AUTH_KIND_NONE: { label: t('mcp.auth_none'), tone: 'ghost' },
  MCP_SERVER_AUTH_KIND_BEARER: { label: t('mcp.auth_bearer'), tone: 'neutral' },
  MCP_SERVER_AUTH_KIND_HEADER: { label: t('mcp.auth_header'), tone: 'neutral' },
} as const;
