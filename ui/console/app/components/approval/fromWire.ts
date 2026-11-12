import type { Approval } from '#gen/approval/v1/approval_pb';
import type { ApprovalArg, ApprovalCall, ApprovalChannel, ApprovalItem, ApprovalStatus, DecidedVia } from '~/stores/approvals';
import { timestampDate } from '@bufbuild/protobuf/wkt';

const statuses: ApprovalStatus[] = ['pending', 'approved', 'rejected', 'expired'];
const channels: ApprovalChannel[] = ['whatsapp', 'api', 'console'];
const vias: DecidedVia['channel'][] = ['console', 'mcp', 'whatsapp', 'api'];
const row = /^([A-Z_][\w.-]*): (.*)$/i;

// previewCalls splits the server's plain-text preview ("<server> · <tool>", then "key: value" lines) back into calls; "…" means more were cut.
export function previewCalls(preview: string): { calls: ApprovalCall[], more: boolean } {
  const calls: ApprovalCall[] = [];
  let more = false;
  for (const line of preview.split('\n')) {
    if (!line) {
      continue;
    }
    if (line === '…') {
      more = true;
      continue;
    }
    const m = row.exec(line);
    if (m) {
      if (!calls.length) {
        calls.push({ title: '', args: [] });
      }
      calls.at(-1)!.args.push({ label: m[1]!, value: m[2]! });
    } else {
      calls.push({ title: line, args: [] });
    }
  }
  return { calls, more };
}

// toApprovalItem maps an ApprovalService approval onto the console's approval shape, naming its agent from the loaded agents.
export function toApprovalItem(a: Approval, agentOf: (id: string) => { name: string, slug: string } | undefined): ApprovalItem {
  const { calls, more } = previewCalls(a.preview);
  const agent = agentOf(a.agentId);
  const args: ApprovalArg[] = calls.flatMap(c => c.args);
  const via = vias.find(v => v === a.decidedVia);
  return {
    approval: {
      id: a.id,
      tool: a.tools[0] ?? '',
      title: calls[0]?.title || a.tools[0] || '',
      args,
      calls,
      more,
      preview: a.preview,
      status: statuses.find(s => s === a.status) ?? 'pending',
      requestedBy: a.requesterLabel || undefined,
      requestedById: a.requesterEndUserId || undefined,
      decidedBy: a.decidedByLabel || undefined,
      decidedById: a.decidedBy || undefined,
      decidedVia: via ? { channel: via } : undefined,
      reason: a.decisionText || undefined,
      expiresAt: a.expiresAt ? timestampDate(a.expiresAt) : undefined,
      decidedAt: a.decidedAt ? timestampDate(a.decidedAt) : undefined,
    },
    agentId: a.agentId,
    agentName: agent?.name ?? a.agentId,
    agentSlug: agent?.slug,
    threadId: a.threadId,
    runId: a.runId,
    channel: channels.find(c => c === a.channel) ?? 'console',
    askedAt: a.createdAt ? timestampDate(a.createdAt) : new Date(0),
  };
}
