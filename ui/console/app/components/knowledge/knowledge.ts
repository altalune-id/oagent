import type { KnowledgeStatus } from '~/stores/knowledge';

export const knowledgeTone = { queued: 'neutral', crawling: 'info', ready: 'success', failed: 'error' } as const satisfies Record<KnowledgeStatus, string>;

export const maxPagesLimit = 500;

// websiteValid reports whether a knowledge website is a full http or https address.
export function websiteValid(url: string): boolean {
  try {
    const u = new URL(url.trim());
    return (u.protocol === 'https:' || u.protocol === 'http:') && u.host.includes('.');
  } catch {
    return false;
  }
}

// maxPagesValid reports whether a crawl page limit is a whole number in range.
export function maxPagesValid(n: number): boolean {
  return Number.isInteger(n) && n >= 1 && n <= maxPagesLimit;
}
