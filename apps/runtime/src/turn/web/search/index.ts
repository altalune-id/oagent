import type { SearchKind } from '../../../contract/index.ts';
import { brave } from './brave.ts';
import { exa } from './exa.ts';

export type SearchResult = {
  title: string,
  url: string,
  snippet: string,
  publishedAt?: string,
};

export interface SearchDriver {
  search: (query: string, apiKey: string, signal: AbortSignal) => Promise<SearchResult[]>;
}

export const drivers: Record<SearchKind, SearchDriver> = { brave, exa };
