import type { KnowledgeDocument, KnowledgeSource } from '~/stores/knowledge';
import { describe, expect, it } from 'vitest';
import { providers } from '~~/test/fixtures/runtime';
import { ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { defaultName, documentState, embeddingChoices, failureText, httpsAddress, reembeds, sourceState } from './knowledge';

const source = (over: Partial<KnowledgeSource>): KnowledgeSource => ({ id: 'Ks', orgId: 'o', projectId: 'p', kind: 'upload', backend: 'pgvector', name: 'n', seedUrl: '', status: 'idle', documents: 0, ready: 0, failed: 0, usedBy: [], ...over });
const doc = (over: Partial<KnowledgeDocument>): KnowledgeDocument => ({ id: 'Kd', sourceId: 'Ks', title: 't', url: '', status: 'ready', pages: 0, ...over });

describe('knowledge helpers', () => {
  it('accepts only a full https address with a host', () => {
    expect(httpsAddress(' https://help.example.com/docs ')).toBe(true);
    expect(httpsAddress('http://help.example.com')).toBe(false);
    expect(httpsAddress('help.example.com/docs')).toBe(false);
    expect(httpsAddress('https://')).toBe(false);
  });

  it('names a website source from its host and path, without a trailing slash', () => {
    expect(defaultName('https://help.example.com/docs/')).toBe('help.example.com/docs');
    expect(defaultName('https://help.example.com')).toBe('help.example.com');
    expect(defaultName('not a url')).toBe('');
    expect(defaultName(`https://a.example.com/${'p'.repeat(300)}`)).toHaveLength(200);
  });

  it('shows a source as Crawling, Failed, Ready, Reading or No documents, never idle', () => {
    expect(sourceState(source({ status: 'crawling' }))).toEqual({ tone: 'info', key: 'knowledge.source_crawling' });
    expect(sourceState(source({ status: 'failed' }))).toEqual({ tone: 'error', key: 'knowledge.source_failed' });
    expect(sourceState(source({ documents: 3, ready: 1 }))).toEqual({ tone: 'success', key: 'knowledge.source_ready' });
    expect(sourceState(source({ documents: 5, ready: 2, failed: 3 }))).toEqual({ tone: 'warning', key: 'knowledge.source_ready_failed', params: { n: 3 } });
    expect(sourceState(source({ documents: 2 }))).toEqual({ tone: 'info', key: 'knowledge.source_reading' });
    expect(sourceState(source({ documents: 2, failed: 2 }))).toEqual({ tone: 'error', key: 'knowledge.source_failed' });
    expect(sourceState(source({}))).toEqual({ tone: 'neutral', key: 'knowledge.source_empty' });
  });

  it('shows a document waiting for an embedding model as a warning', () => {
    expect(documentState(doc({ status: 'uploading' }))).toEqual({ tone: 'info', key: 'knowledge.doc_uploading' });
    expect(documentState(doc({ status: 'pending' }))).toEqual({ tone: 'neutral', key: 'knowledge.doc_pending' });
    expect(documentState(doc({ status: 'pending', failure: { code: 'KNW011', reason: '' } }))).toEqual({ tone: 'warning', key: 'knowledge.doc_waiting_model' });
    expect(documentState(doc({ status: 'pending' }), false)).toEqual({ tone: 'warning', key: 'knowledge.doc_waiting_model' });
    expect(documentState(doc({ status: 'ready' }), false)).toEqual({ tone: 'success', key: 'knowledge.doc_ready' });
    expect(documentState(doc({ status: 'processing' }))).toEqual({ tone: 'info', key: 'knowledge.doc_processing' });
    expect(documentState(doc({ status: 'ready' }))).toEqual({ tone: 'success', key: 'knowledge.doc_ready' });
    expect(documentState(doc({ status: 'failed' }))).toEqual({ tone: 'error', key: 'knowledge.doc_failed' });
  });

  it('offers models marked for embeddings on OpenAI-compatible providers, flagging the unpriced ones', () => {
    const list = providers();
    list[0]!.models.push({ id: 'text-embedding-3-small', vision: false, embedding: true, prices: { inputMicros: 20_000n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n } });
    list[1]!.models.push({ id: 'nomic-embed', vision: false, embedding: true });
    list[2]!.models.push({ id: 'claude-embed', vision: false, embedding: true });
    expect(list[2]!.kind).toBe(ModelProviderKind.ANTHROPIC);
    expect(embeddingChoices(list)).toEqual([
      { providerId: list[0]!.id, providerName: 'OpenRouter', model: 'text-embedding-3-small', priced: true },
      { providerId: list[1]!.id, providerName: 'Groq', model: 'nomic-embed', priced: false },
    ]);
  });

  it('asks to re-embed only when an existing model changes while sources exist', () => {
    const now = { providerId: 'P1', model: 'a', dims: 1536 };
    expect(reembeds(null, { providerId: 'P1', model: 'a' }, 3)).toBe(false);
    expect(reembeds(now, { providerId: 'P1', model: 'a' }, 3)).toBe(false);
    expect(reembeds(now, { providerId: 'P1', model: 'b' }, 0)).toBe(false);
    expect(reembeds(now, { providerId: 'P2', model: 'a' }, 1)).toBe(true);
  });

  it('words a failure by its reason, then its code, then a generic line', () => {
    const t = (k: string) => `t:${k}`;
    const known = (...keys: string[]) => (k: string) => keys.includes(k);
    expect(failureText({ code: 'KNW014', reason: 'robots_disallowed' }, t, known('apperror.KNW014_robots_disallowed', 'apperror.KNW014'))).toBe('t:apperror.KNW014_robots_disallowed');
    expect(failureText({ code: 'KNW014', reason: 'new_reason' }, t, known('apperror.KNW014'))).toBe('t:apperror.KNW014');
    expect(failureText({ code: 'XYZ999', reason: '' }, t, known())).toBe('t:knowledge.failed_unknown');
    expect(failureText({ code: 'KNW010', reason: 'chunks_per_document' }, t, known('knowledge.fail_KNW010_chunks_per_document', 'apperror.KNW010_chunks_per_document'))).toBe('t:knowledge.fail_KNW010_chunks_per_document');
  });
});
