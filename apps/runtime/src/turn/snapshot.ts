import type { Snapshot } from '@strands-agents/sdk';
import type { TurnRequest } from '../contract/index.ts';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { Agent } from '@strands-agents/sdk';
import { OpenAIModel } from '@strands-agents/sdk/models/openai';

import { TurnRefusedError } from '../contract/index.ts';

export const IMAGE_NOTE = '[image removed]';

function sdkVersion(): string {
  let dir = new URL('.', import.meta.resolve('@strands-agents/sdk'));
  for (let i = 0; i < 6; i++) {
    try {
      const pkg = JSON.parse(readFileSync(fileURLToPath(new URL('package.json', dir)), 'utf8')) as { name?: string, version?: string };
      if (pkg.name === '@strands-agents/sdk' && pkg.version) {
        return pkg.version;
      }
    } catch {}
    dir = new URL('..', dir);
  }
  throw new Error('strands sdk version not found');
}

export const SDK_VERSION = sdkVersion();

type Block = Record<string, unknown>;
type MessageJson = { role: string, content: Block[] };

function stripBlocks(blocks: Block[]): Block[] {
  return blocks.map((block) => {
    if ('image' in block) {
      return { text: IMAGE_NOTE };
    }
    if ('toolResult' in block) {
      const tr = block.toolResult as { content?: Block[] };
      return { ...block, toolResult: { ...tr, content: stripBlocks(tr.content ?? []) } };
    }
    return block;
  });
}

function isRefusal(message: MessageJson | undefined, refusal: string): boolean {
  return message?.role === 'assistant'
    && message.content.length === 1
    && message.content[0]!.text === refusal;
}

export function cleanSnapshot(snapshot: Snapshot, refusal?: string): Snapshot {
  const data = structuredClone(snapshot.data) as Record<string, unknown>;
  const messages = (data.messages ?? []) as MessageJson[];
  const kept = refusal !== undefined && isRefusal(messages.at(-1), refusal) ? messages.slice(0, -1) : messages;
  data.messages = kept.map(m => ({ ...m, content: stripBlocks(m.content) }));
  return { ...snapshot, data: data as Snapshot['data'] };
}

export type SnapshotTags = {
  thread: string,
  agentVersion: number,
};

export function tagsMatch(snapshot: Record<string, unknown>, want: SnapshotTags): boolean {
  const app = snapshot.appData as Record<string, unknown> | undefined;
  return app?.thread === want.thread && app.agentVersion === want.agentVersion && app.sdk === SDK_VERSION;
}

export function checkSnapshot(request: TurnRequest): void {
  if (request.snapshot === null) {
    return;
  }
  if (!tagsMatch(request.snapshot, { thread: request.ids.thread, agentVersion: request.ids.agentVersion })) {
    throw new TurnRefusedError(422, 'snapshot_unloadable');
  }
  try {
    const probe = new Agent({ model: new OpenAIModel({ api: 'chat', modelId: 'snapshot-check', apiKey: 'unused' }), sandbox: false, printer: false });
    probe.loadSnapshot(request.snapshot as unknown as Snapshot);
  } catch {
    throw new TurnRefusedError(422, 'snapshot_unloadable');
  }
}
