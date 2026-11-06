import { createHash } from 'node:crypto';
import { OpenAIModel } from '@strands-agents/sdk/models/openai';
import OpenAI from 'openai';

import { guardedFetch } from './egress.ts';

const DEFAULT_MAX = 256;
const DEFAULT_IDLE_MS = 15 * 60_000;

export interface ModelSpec {
  baseUrl: string;
  apiKey: string;
  modelId: string;
  maxTokens?: number;
}

export interface ModelClientsOptions {
  max?: number;
  idleMs?: number;
}

interface Entry {
  client: OpenAI;
  lastUsed: number;
}

export class ModelClients {
  readonly max: number;
  readonly idleMs: number;
  private readonly entries = new Map<string, Entry>();

  constructor(options: ModelClientsOptions = {}) {
    this.max = options.max ?? DEFAULT_MAX;
    this.idleMs = options.idleMs ?? DEFAULT_IDLE_MS;
  }

  get size(): number {
    return this.entries.size;
  }

  keys(): string[] {
    return [...this.entries.keys()];
  }

  client(project: string, spec: ModelSpec): OpenAI {
    const now = Date.now();
    this.dropIdle(now);
    const key = `${project}:${createHash('sha256').update(JSON.stringify([spec.baseUrl, spec.apiKey])).digest('hex')}`;
    const hit = this.entries.get(key);
    if (hit) {
      this.entries.delete(key);
      hit.lastUsed = now;
      this.entries.set(key, hit);
      return hit.client;
    }
    const client = new OpenAI({
      baseURL: spec.baseUrl,
      apiKey: spec.apiKey,
      organization: null,
      project: null,
      maxRetries: 0,
      fetch: guardedFetch(spec.baseUrl, { header: 'authorization', value: `Bearer ${spec.apiKey}`, secrets: [`Bearer ${spec.apiKey}`, spec.apiKey] }),
    });
    this.entries.set(key, { client, lastUsed: now });
    while (this.entries.size > this.max) {
      this.entries.delete(this.entries.keys().next().value!);
    }
    return client;
  }

  model(project: string, spec: ModelSpec, temperature: number): OpenAIModel {
    return new OpenAIModel({
      api: 'chat',
      client: this.client(project, spec),
      modelId: spec.modelId,
      temperature,
      ...(spec.maxTokens !== undefined && { maxTokens: spec.maxTokens }),
    });
  }

  private dropIdle(now: number): void {
    for (const [key, entry] of this.entries) {
      if (now - entry.lastUsed <= this.idleMs) {
        return;
      }
      this.entries.delete(key);
    }
  }
}
