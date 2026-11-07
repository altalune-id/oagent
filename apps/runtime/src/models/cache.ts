const DEFAULT_MAX = 256;
const DEFAULT_IDLE_MS = 15 * 60_000;

export type CacheOptions = {
  max?: number,
  idleMs?: number,
};

type Entry<T> = {
  client: T,
  apiKey: string,
  lastUsed: number,
};

export class ClientCache<T> {
  readonly max: number;
  readonly idleMs: number;
  private readonly entries = new Map<string, Entry<T>>();

  constructor(options: CacheOptions = {}) {
    this.max = options.max ?? DEFAULT_MAX;
    this.idleMs = options.idleMs ?? DEFAULT_IDLE_MS;
  }

  get size(): number {
    return this.entries.size;
  }

  keys(): string[] {
    return [...this.entries.keys()];
  }

  get(key: string, apiKey: string, create: () => T): T {
    const now = Date.now();
    this.dropIdle(now);
    const hit = this.entries.get(key);
    this.entries.delete(key);
    if (hit?.apiKey === apiKey) {
      hit.lastUsed = now;
      this.entries.set(key, hit);
      return hit.client;
    }
    const client = create();
    this.entries.set(key, { client, apiKey, lastUsed: now });
    while (this.entries.size > this.max) {
      this.entries.delete(this.entries.keys().next().value!);
    }
    return client;
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
