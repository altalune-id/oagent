import type { ToolCaller } from './invite';

import { describe, expect, it } from 'vitest';
import { confirmDraft, discardDraft, EXPIRED_MESSAGE, isExpired, parseDraft } from './invite';

const now = new Date('2026-10-13T09:00:00Z');

const result = {
  invite: { id: 'inv1', email: 'dina@x.io', role: 'member', status: 'draft', expiresAt: '2026-10-13T09:15:00Z', invitedByName: 'Olga' },
  orgId: 'org1',
  orgName: 'Acme',
};

function caller(reply: Awaited<ReturnType<ToolCaller['callServerTool']>>) {
  const calls: { name: string, arguments: Record<string, unknown> }[] = [];
  const app: ToolCaller = {
    callServerTool: async (p) => {
      calls.push(p);
      return reply;
    },
  };
  return { app, calls };
}

describe('parseDraft', () => {
  it('reads the draft as protojson sends it', () => {
    expect(parseDraft(result)).toEqual({
      id: 'inv1',
      email: 'dina@x.io',
      role: 'Member',
      orgId: 'org1',
      orgName: 'Acme',
      invitedBy: 'Olga',
      expiresAt: new Date('2026-10-13T09:15:00Z'),
    });
  });

  it('reads a malformed result as no draft', () => {
    expect(parseDraft(undefined)).toBeNull();
    expect(parseDraft({})).toBeNull();
    expect(parseDraft({ invite: { email: 'x' }, orgId: 'o' })).toBeNull();
    expect(parseDraft({ invite: { id: 'i', email: 'x' } })).toBeNull();
  });
});

describe('isExpired', () => {
  it('compares the expiry with now', () => {
    const d = parseDraft(result)!;
    expect(isExpired(d, now)).toBe(false);
    expect(isExpired(d, new Date('2026-10-13T09:15:00Z'))).toBe(true);
    expect(isExpired({ ...d, expiresAt: null }, now)).toBe(false);
  });
});

describe('confirmDraft', () => {
  it('calls the app-only confirm tool and returns no link', async () => {
    const { app, calls } = caller({ structuredContent: { invite: { email: 'dina@x.io' } } });
    const out = await confirmDraft(app, parseDraft(result)!, now);
    expect(calls).toEqual([{ name: 'member_invite_confirm', arguments: { org_id: 'org1', invite_id: 'inv1' } }]);
    expect(out).toEqual({ state: 'sent', email: 'dina@x.io' });
  });

  it('does not call the server for an expired draft', async () => {
    const { app, calls } = caller({});
    const out = await confirmDraft(app, parseDraft(result)!, new Date('2026-10-13T10:00:00Z'));
    expect(calls).toEqual([]);
    expect(out).toEqual({ state: 'expired', message: EXPIRED_MESSAGE });
    expect(EXPIRED_MESSAGE).toBe('This draft expired; ask the assistant again.');
  });

  it('reads INV003 from the server as expired', async () => {
    const { app } = caller({ isError: true, content: [{ type: 'text', text: '{"code":"INV003","message":"This draft expired"}' }] });
    expect(await confirmDraft(app, parseDraft(result)!, now)).toEqual({ state: 'expired', message: EXPIRED_MESSAGE });
  });

  it('shows any other error message', async () => {
    const { app } = caller({ isError: true, content: [{ type: 'text', text: '{"code":"GEN007","message":"Forbidden"}' }] });
    expect(await confirmDraft(app, parseDraft(result)!, now)).toEqual({ state: 'error', message: 'Forbidden' });
    const bad = caller({ isError: true, content: [{ type: 'text', text: 'not json' }] });
    expect(await confirmDraft(bad.app, parseDraft(result)!, now)).toEqual({ state: 'error', message: 'not json' });
    const thrown: ToolCaller = { callServerTool: async () => {
      throw new Error('offline');
    } };
    expect(await confirmDraft(thrown, parseDraft(result)!, now)).toEqual({ state: 'error', message: 'Error: offline' });
  });
});

describe('discardDraft', () => {
  it('calls the app-only discard tool', async () => {
    const { app, calls } = caller({ structuredContent: {} });
    expect(await discardDraft(app, parseDraft(result)!)).toEqual({ state: 'discarded' });
    expect(calls).toEqual([{ name: 'member_invite_discard', arguments: { org_id: 'org1', invite_id: 'inv1' } }]);
  });

  it('shows the error', async () => {
    const { app } = caller({ isError: true, content: [{ type: 'text', text: '{"code":"GEN005","message":"Invite not found"}' }] });
    expect(await discardDraft(app, parseDraft(result)!)).toEqual({ state: 'error', message: 'Invite not found' });
  });
});
