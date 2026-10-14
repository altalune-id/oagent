import { roleLabel } from './members';

export const EXPIRED_MESSAGE = 'This draft expired; ask the assistant again.';

export interface Draft {
  id: string;
  email: string;
  role: string;
  orgId: string;
  orgName: string;
  invitedBy: string;
  expiresAt: Date | null;
}

interface ToolResult {
  isError?: boolean;
  content?: { type: string, text?: string }[];
  structuredContent?: Record<string, unknown>;
}

// ToolCaller is the part of the MCP App bridge the invite view uses.
export interface ToolCaller {
  callServerTool: (params: { name: string, arguments: Record<string, unknown> }) => Promise<ToolResult>;
}

export type Outcome
  = | { state: 'sent', email: string }
    | { state: 'discarded' }
    | { state: 'expired', message: string }
    | { state: 'error', message: string };

export function parseDraft(data: Record<string, unknown> | undefined): Draft | null {
  const invite = data?.invite as Record<string, unknown> | undefined;
  const id = typeof invite?.id === 'string' ? invite.id : '';
  const orgId = typeof data?.orgId === 'string' ? data.orgId : '';
  if (!id || !orgId) {
    return null;
  }
  const expires = typeof invite?.expiresAt === 'string' ? new Date(invite.expiresAt) : null;
  return {
    id,
    email: String(invite?.email ?? ''),
    role: roleLabel(invite?.role as string | undefined),
    orgId,
    orgName: String(data?.orgName ?? ''),
    invitedBy: String(invite?.invitedByName ?? ''),
    expiresAt: expires && !Number.isNaN(expires.getTime()) ? expires : null,
  };
}

export function isExpired(d: Draft, now: Date): boolean {
  return d.expiresAt !== null && now.getTime() >= d.expiresAt.getTime();
}

export async function confirmDraft(app: ToolCaller, d: Draft, now: Date): Promise<Outcome> {
  if (isExpired(d, now)) {
    return { state: 'expired', message: EXPIRED_MESSAGE };
  }
  const res = await call(app, 'member_invite_confirm', d);
  if (!res.ok) {
    return res.outcome;
  }
  const invite = res.data.invite as Record<string, unknown> | undefined;
  return { state: 'sent', email: String(invite?.email ?? d.email) };
}

export async function discardDraft(app: ToolCaller, d: Draft): Promise<Outcome> {
  const res = await call(app, 'member_invite_discard', d);
  return res.ok ? { state: 'discarded' } : res.outcome;
}

type Called = { ok: true, data: Record<string, unknown> } | { ok: false, outcome: Outcome };

async function call(app: ToolCaller, name: string, d: Draft): Promise<Called> {
  let res: ToolResult;
  try {
    res = await app.callServerTool({ name, arguments: { org_id: d.orgId, invite_id: d.id } });
  } catch (err) {
    return { ok: false, outcome: { state: 'error', message: String(err) } };
  }
  if (!res.isError) {
    return { ok: true, data: res.structuredContent ?? {} };
  }
  return { ok: false, outcome: failure(res.content?.find(c => c.type === 'text')?.text ?? '') };
}

function failure(text: string): Outcome {
  try {
    const payload = JSON.parse(text) as { code?: string, message?: string };
    if (payload.code === 'INV003') {
      return { state: 'expired', message: EXPIRED_MESSAGE };
    }
    return { state: 'error', message: payload.message || text };
  } catch {
    return { state: 'error', message: text };
  }
}
