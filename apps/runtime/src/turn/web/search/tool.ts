import type { SearchKind } from '../../../contract/index.ts';
import type { WebTurn } from '../call.ts';
import { tool } from '@strands-agents/sdk';
import { z } from 'zod';

import { addSearchCall, searchFits } from '../../limits.ts';
import { runCall } from '../call.ts';
import { WebToolError } from '../errors.ts';
import { drivers } from './index.ts';

export type SearchSpec = {
  kind: SearchKind,
  apiKey: string,
};

export function searchTool(turn: WebTurn, spec: SearchSpec) {
  return tool({
    name: 'web_search',
    description: 'Search the web. Returns up to 5 results, each with a title, url, snippet and optional publishedAt. Use web_fetch to read a result.',
    inputSchema: z.strictObject({ query: z.string().min(1).max(400).describe('The search query') }),
    callback: (input, ctx) => runCall(turn, 'web_search', ctx, turn.timeoutMs, async (signal) => {
      if (!searchFits(turn.usage, turn.budget, turn.calls.searchesInFlight)) {
        throw new WebToolError('over_budget');
      }
      turn.calls.searchesInFlight++;
      let billed = false;
      try {
        const results = await drivers[spec.kind].search(input.query, spec.apiKey, signal);
        billed = true;
        return { text: JSON.stringify(results) };
      } catch (error) {
        billed = error instanceof WebToolError && error.billed;
        throw error;
      } finally {
        turn.calls.searchesInFlight--;
        if (billed) {
          addSearchCall(turn.usage);
        }
      }
    }),
  });
}
