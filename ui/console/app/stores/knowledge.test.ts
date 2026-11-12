import type { UploadTransport } from '#base/components/ui/upload';
import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { allowedTypes, docPB, documentPBs, fakeKnowledgeClient, orgId, projectId } from '~~/test/fixtures/knowledge';
import { UploadFailedError } from '#base/components/ui/upload';
import { KnowledgeBackend, KnowledgeDocumentSchema, KnowledgeDocumentStatus } from '#gen/knowledge/v1/knowledge_pb';
import { connectError, deferred, setKnownKeys } from '#test/nuxt-globals';
import { acceptFor, contentTypeOf, failureKeys, needsPolling, pollIntervalMs, toDocument, useKnowledgeStore } from './knowledge';

const uploadSource = 'Ks1Upl0adQ8sWx3YcLm7D';
const pdf = (name = 'policy.pdf', bytes = 4) => new File(['x'.repeat(bytes)], name, { type: 'application/pdf' });

let client: ReturnType<typeof fakeKnowledgeClient>;

beforeEach(() => {
  setActivePinia(createPinia());
  client = fakeKnowledgeClient();
  setKnownKeys();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('knowledge mapping', () => {
  it('maps a file name to an allowed content type by extension, and lists the extensions to accept', () => {
    expect(['a.PDF', 'b.docx', 'c.md', 'd.markdown', 'e.txt', 'f.png', 'noext'].map(n => contentTypeOf(n, allowedTypes))).toEqual([
      'application/pdf',
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      'text/markdown',
      'text/markdown',
      'text/plain',
      '',
      '',
    ]);
    expect(contentTypeOf('a.pdf', ['text/plain'])).toBe('');
    expect(acceptFor(allowedTypes)).toBe('.pdf,.docx,.md,.markdown,.txt');
    expect(acceptFor(['text/plain', 'application/x-unknown'])).toBe('.txt');
  });

  it('looks up a failure by its reason first, then its code', () => {
    expect(failureKeys({ code: 'KNW014', reason: 'robots_disallowed' })).toEqual(['apperror.KNW014_robots_disallowed', 'apperror.KNW014']);
    expect(failureKeys({ code: 'KNW005', reason: '' })).toEqual(['apperror.KNW005']);
  });
});

describe('useKnowledgeStore: sources', () => {
  it('loads sources, storage, enabled backends and the embedding model', async () => {
    const store = useKnowledgeStore();
    await store.load(projectId);
    expect(client.listKnowledgeSources).toHaveBeenCalledWith({ projectId });
    expect(store.sources.map(s => [s.name, s.kind, s.status])).toEqual([['Refund policy', 'upload', 'idle'], ['help.example.com/docs', 'url', 'crawling'], ['shop.example.com', 'url', 'failed']]);
    expect(store.sources[0]).toMatchObject({ documents: 3, ready: 2, failed: 1, usedBy: [{ id: 'Ag7Rf2Tq9Lm4Xw8Pz3Kc1', name: 'Refund Triage' }] });
    expect(store.sources[2]!.failure).toEqual({ code: 'KNW014', reason: 'robots_disallowed' });
    expect(store.sources[0]!.updatedAt).toEqual(new Date('2026-11-10T08:00:00Z'));
    expect(store.storageConfigured).toBe(true);
    expect([store.maxUploadBytes, store.allowedContentTypes, store.uploadAccept]).toEqual([10_485_760, allowedTypes, '.pdf,.docx,.md,.markdown,.txt']);
    expect(store.enabledBackends).toEqual(['pgvector']);
    expect(store.on).toBe(true);
    expect(store.embedding).toEqual({ providerId: 'Pv6tQm2ZxR8wKp3VbNc4K', model: 'openai/text-embedding-3-small', dims: 1536, providerMissing: false });
    await store.ensureLoaded(projectId);
    expect(client.listKnowledgeSources).toHaveBeenCalledOnce();
  });

  it('reads knowledge off (no backend) and no embedding model, and shows a load error', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ sources: [], storageConfigured: false, enabledBackends: [] });
    const store = useKnowledgeStore();
    await store.load(projectId);
    expect([store.on, store.storageConfigured, store.embedding]).toEqual([false, false, null]);
    setKnownKeys('apperror.GEN900');
    client.listKnowledgeSources.mockRejectedValueOnce(connectError('GEN900'));
    await store.load(projectId);
    expect(store.error?.message).toBe('apperror.GEN900');
    expect(store.loading).toBe(false);
  });

  it('creates website and upload sources and adds them first', async () => {
    const store = useKnowledgeStore();
    await store.load(projectId);
    const url = await store.createUrlSource(orgId, projectId, 'Docs', 'https://docs.example.com');
    expect(client.createURLSource).toHaveBeenCalledWith({ orgId, projectId, name: 'Docs', seedUrl: 'https://docs.example.com' });
    const upload = await store.createUploadSource(orgId, projectId, 'Manuals');
    expect(client.createUploadSource).toHaveBeenCalledWith({ orgId, projectId, name: 'Manuals' });
    expect(store.sources.slice(0, 2).map(s => s.id)).toEqual([upload.id, url.id]);
  });

  it('crawls again, deletes, and drops a deleted source\'s documents', async () => {
    const store = useKnowledgeStore();
    await store.load(projectId);
    await store.refresh('Ks3Fa1ledRp5Nw2Qk7Vb4');
    expect(store.byId('Ks3Fa1ledRp5Nw2Qk7Vb4')).toMatchObject({ status: 'crawling', failure: undefined });
    await store.loadDocuments(uploadSource);
    await store.removeSource(uploadSource);
    expect(client.deleteKnowledgeSource).toHaveBeenCalledWith({ id: uploadSource });
    expect(store.byId(uploadSource)).toBeUndefined();
    expect(store.documents[uploadSource]).toBeUndefined();
  });

  it('sets the embedding model and keeps what the server answered', async () => {
    const store = useKnowledgeStore();
    await store.setEmbedding(projectId, 'Pv1', 'nomic-embed');
    expect(client.setEmbeddingModel).toHaveBeenCalledWith({ projectId, providerId: 'Pv1', model: 'nomic-embed' });
    expect(store.embedding).toEqual({ providerId: 'Pv1', model: 'nomic-embed', dims: 1024, providerMissing: false });
  });

  it('runs a test search and keeps the hits as plain data', async () => {
    const store = useKnowledgeStore();
    expect(await store.search('Ks1', ' refund time ')).toEqual([{ text: 'Refunds take 5 working days.', title: 'refund-policy.pdf', heading: 'Timing', page: 2, url: '', score: 0.8312 }]);
    expect(client.testSearch).toHaveBeenCalledWith({ sourceId: 'Ks1', query: 'refund time' });
  });
});

describe('useKnowledgeStore: documents', () => {
  it('lists a source\'s documents with status and failure, and deletes one', async () => {
    const store = useKnowledgeStore();
    await store.loadDocuments(uploadSource);
    expect(store.documents[uploadSource]!.map(d => [d.title, d.status, d.failure?.code])).toEqual([['refund-policy.pdf', 'ready', undefined], ['faq.md', 'pending', 'KNW011'], ['scan.pdf', 'failed', 'KNW005']]);
    await store.removeDocument(uploadSource, 'Kd3Fail0Lm4Xw8Pz3Kc1q');
    expect(client.deleteKnowledgeDocument).toHaveBeenCalledWith({ id: 'Kd3Fail0Lm4Xw8Pz3Kc1q' });
    expect(store.documents[uploadSource]!.map(d => d.title)).toEqual(['refund-policy.pdf', 'faq.md']);
  });

  it('reads a document\'s size and passages, leaving them out while the server sends 0', () => {
    const pb = documentPBs()[0]!;
    expect(toDocument(pb)).toMatchObject({ bytes: undefined, passages: undefined });
    expect(toDocument(create(KnowledgeDocumentSchema, { ...pb, sizeBytes: 2048n, chunkCount: 7 }))).toMatchObject({ bytes: 2048, passages: 7 });
  });

  it('keeps only the latest documents answer', async () => {
    const first = deferred<{ documents: unknown[] }>();
    client.listKnowledgeDocuments.mockReturnValueOnce(first.promise);
    const store = useKnowledgeStore();
    const slow = store.loadDocuments(uploadSource);
    client.listKnowledgeDocuments.mockResolvedValueOnce({ documents: [] });
    await store.loadDocuments(uploadSource);
    first.resolve({ documents: documentPBs() });
    await slow;
    expect(store.documents[uploadSource]).toEqual([]);
  });

  it('shows a documents load error', async () => {
    setKnownKeys('apperror.KNW001');
    client.listKnowledgeDocuments.mockRejectedValueOnce(connectError('KNW001'));
    const store = useKnowledgeStore();
    await store.loadDocuments(uploadSource);
    expect(store.documentsError?.message).toBe('apperror.KNW001');
  });
});

describe('useKnowledgeStore: upload', () => {
  it('presigns, PUTs the file itself to the presigned URL, then completes; no RPC carries the bytes', async () => {
    const calls: string[] = [];
    client.beginUpload.mockImplementationOnce(async (req: { sourceId: string, filename: string }) => {
      calls.push('begin');
      return { document: docPB({ id: 'Kd-policy', sourceId: req.sourceId, title: req.filename, status: KnowledgeDocumentStatus.UPLOADING }), upload: { url: 'https://bucket.example.com/k?sig=1', method: 'PUT', headers: { 'Content-Type': 'application/pdf' } } };
    });
    client.completeUpload.mockImplementationOnce(async ({ documentId }: { documentId: string }) => {
      calls.push('complete');
      return { document: docPB({ id: documentId, sourceId: uploadSource, title: 'policy.pdf', status: KnowledgeDocumentStatus.PENDING }) };
    });
    const file = pdf();
    const put = vi.fn<UploadTransport>(async (_url, _file, _headers, progress) => {
      calls.push('put');
      progress(0.5);
    });
    const store = useKnowledgeStore();
    await store.load(projectId);
    await store.upload(uploadSource, [file], put);
    expect(calls).toEqual(['begin', 'put', 'complete']);
    expect(client.beginUpload).toHaveBeenCalledWith({ sourceId: uploadSource, filename: 'policy.pdf', contentType: 'application/pdf', sizeBytes: 4n });
    expect(put).toHaveBeenCalledWith('https://bucket.example.com/k?sig=1', file, { 'Content-Type': 'application/pdf' }, expect.any(Function));
    expect(client.completeUpload).toHaveBeenCalledWith({ documentId: 'Kd-policy' });
    for (const call of [...client.beginUpload.mock.calls, ...client.completeUpload.mock.calls]) {
      expect(Object.values(call[0] as object)).not.toContain(file);
    }
    expect(store.uploads[uploadSource]).toEqual([expect.objectContaining({ name: 'policy.pdf', bytes: 4, state: 'done', progress: 1 })]);
    expect(store.documents[uploadSource]!.map(d => [d.id, d.status])).toEqual([['Kd-policy', 'pending']]);
  });

  it('keeps going after a failed PUT and marks only that file failed', async () => {
    const put = vi.fn<UploadTransport>().mockRejectedValueOnce(new UploadFailedError(403)).mockResolvedValueOnce();
    const store = useKnowledgeStore();
    await store.upload(uploadSource, [pdf('a.pdf'), pdf('b.pdf')], put);
    expect(store.uploads[uploadSource]!.map(u => [u.name, u.state, u.error])).toEqual([['a.pdf', 'failed', 'knowledge.upload_put_failed'], ['b.pdf', 'done', undefined]]);
    expect(client.completeUpload).toHaveBeenCalledOnce();
    expect(client.deleteKnowledgeDocument).toHaveBeenCalledWith({ id: 'Kd-a.pdf' });
    expect(store.documents[uploadSource]!.map(d => d.id)).toEqual(['Kd-b.pdf']);
  });

  it('shows a server refusal (KNW009, KNW010 with its reason) on the file row', async () => {
    setKnownKeys('apperror.KNW009', 'apperror.KNW010_documents_per_source');
    client.beginUpload.mockRejectedValueOnce(connectError('KNW009')).mockRejectedValueOnce(connectError('KNW010', { reason: 'documents_per_source', max: '100' }));
    const store = useKnowledgeStore();
    await store.upload(uploadSource, [pdf('a.pdf'), pdf('b.pdf')], vi.fn());
    expect(store.uploads[uploadSource]!.map(u => u.error)).toEqual(['apperror.KNW009', 'apperror.KNW010_documents_per_source|{"max":"100"}']);
  });

  it('refuses a signed upload that isn\'t a PUT without sending the file', async () => {
    client.beginUpload.mockResolvedValueOnce({ document: docPB({ id: 'Kd-x', sourceId: uploadSource, title: 'x.pdf', status: KnowledgeDocumentStatus.UPLOADING }), upload: { url: 'https://b/k', method: 'POST', headers: { 'Content-Type': 'application/pdf' } } });
    const put = vi.fn<UploadTransport>();
    const store = useKnowledgeStore();
    await store.upload(uploadSource, [pdf('x.pdf')], put);
    expect(put).not.toHaveBeenCalled();
    expect(store.uploads[uploadSource]![0]!.state).toBe('failed');
    expect(client.deleteKnowledgeDocument).toHaveBeenCalledWith({ id: 'Kd-x' });
  });

  it('lists files the control refused, with the reason, and never calls the server for them', () => {
    const store = useKnowledgeStore();
    store.refuse(uploadSource, [{ file: pdf('photo.png'), reason: 'type' }, { file: pdf('huge.pdf', 9), reason: 'size' }]);
    expect(store.uploads[uploadSource]!.map(u => [u.name, u.state, u.reason, u.error])).toEqual([['photo.png', 'failed', 'type', undefined], ['huge.pdf', 'failed', 'size', undefined]]);
    expect(client.beginUpload).not.toHaveBeenCalled();
  });
});

describe('useKnowledgeStore: polling', () => {
  it('polls while a crawl or ingestion is under way, and only when there is a model to ingest with', async () => {
    const store = useKnowledgeStore();
    await store.load(projectId);
    expect(needsPolling(store.sources, store.embedding)).toBe(true);
    const settled = store.sources.map(s => ({ ...s, status: 'idle' as const, documents: s.ready + s.failed }));
    expect(needsPolling(settled, store.embedding)).toBe(false);
    const waiting = settled.map(s => ({ ...s, documents: s.documents + 1 }));
    expect(needsPolling(waiting, store.embedding)).toBe(true);
    expect(needsPolling(waiting, null)).toBe(false);
  });

  it('leaves a source\'s browser uploads out once its documents are loaded', () => {
    const source = { id: 'Ks', orgId, projectId, kind: 'upload' as const, backend: 'pgvector' as const, name: 'n', seedUrl: '', status: 'idle' as const, documents: 2, ready: 1, failed: 0, usedBy: [] };
    const doc = (status: 'uploading' | 'pending' | 'ready') => ({ id: status, sourceId: 'Ks', title: 't', url: '', status, pages: 0 });
    const model = { providerId: 'P', model: 'm', dims: 1536 };
    expect(needsPolling([source], model)).toBe(true);
    expect(needsPolling([source], model, { Ks: [doc('ready'), doc('uploading')] })).toBe(false);
    expect(needsPolling([source], model, { Ks: [doc('ready'), doc('pending')] })).toBe(true);
    expect(needsPolling([{ ...source, ready: 0 }], model, { Ks: [doc('ready')] }), 'the source counts lag the documents').toBe(true);
  });

  it('reloads quietly every interval while busy, with the source\'s documents, and stops once settled', async () => {
    vi.useFakeTimers();
    const store = useKnowledgeStore();
    await store.load(projectId);
    const stop = store.startPolling(projectId, uploadSource);
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(client.listKnowledgeSources).toHaveBeenCalledTimes(2);
    expect(client.listKnowledgeDocuments).toHaveBeenCalledWith({ sourceId: uploadSource });
    client.listKnowledgeSources.mockResolvedValue({ sources: [], storageConfigured: true, enabledBackends: [KnowledgeBackend.PGVECTOR] });
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    await vi.advanceTimersByTimeAsync(pollIntervalMs * 3);
    expect(client.listKnowledgeSources).toHaveBeenCalledTimes(3);
    stop();
  });

  it('loads before it decides, so a poll started on settled state still sees a re-embed', async () => {
    vi.useFakeTimers();
    const store = useKnowledgeStore();
    await store.load(projectId);
    store.sources = store.sources.map(s => ({ ...s, status: 'idle' as const, documents: s.ready + s.failed }));
    client.listKnowledgeSources.mockClear();
    const stop = store.startPolling(projectId);
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(client.listKnowledgeSources).toHaveBeenCalledOnce();
    stop();
  });

  it('ends a retry superseded by a poll with no spinner and no error', async () => {
    setKnownKeys('apperror.GEN900');
    const store = useKnowledgeStore();
    client.listKnowledgeSources.mockRejectedValueOnce(connectError('GEN900'));
    await store.load(projectId);
    expect(store.error).not.toBeNull();
    await store.load(projectId, true);
    expect(store.error, 'a successful poll clears an old load error').toBeNull();
    client.listKnowledgeSources.mockRejectedValueOnce(connectError('GEN900'));
    await store.load(projectId);
    const retry = deferred<unknown>();
    client.listKnowledgeSources.mockReturnValueOnce(retry.promise);
    const loud = store.load(projectId);
    await store.load(projectId, true);
    retry.resolve({ sources: [], storageConfigured: true, enabledBackends: [] });
    await loud;
    expect([store.loading, store.error]).toEqual([false, null]);
    const docs = deferred<unknown>();
    client.listKnowledgeDocuments.mockReturnValueOnce(docs.promise);
    const loudDocs = store.loadDocuments(uploadSource);
    await store.loadDocuments(uploadSource, true);
    docs.resolve({ documents: [] });
    await loudDocs;
    expect([store.documentsLoading, store.documentsError]).toEqual([false, null]);
  });

  it('keeps each owner\'s timer: stopping one poll leaves another running', async () => {
    vi.useFakeTimers();
    const store = useKnowledgeStore();
    await store.load(projectId);
    const stopA = store.startPolling(projectId);
    const stopB = store.startPolling(projectId, uploadSource);
    stopA();
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(client.listKnowledgeDocuments).toHaveBeenCalledWith({ sourceId: uploadSource });
    expect(client.listKnowledgeSources).toHaveBeenCalledTimes(2);
    stopB();
    await vi.advanceTimersByTimeAsync(pollIntervalMs * 2);
    expect(client.listKnowledgeSources).toHaveBeenCalledTimes(2);
  });

  it('polls quietly: no loading flag, and a failed poll keeps the last data and shows no error', async () => {
    vi.useFakeTimers();
    const store = useKnowledgeStore();
    await store.load(projectId);
    await store.loadDocuments(uploadSource);
    const seen: boolean[] = [];
    store.$subscribe(() => seen.push(store.loading || store.documentsLoading));
    client.listKnowledgeSources.mockRejectedValueOnce(connectError('GEN900'));
    client.listKnowledgeDocuments.mockRejectedValueOnce(connectError('GEN900'));
    const stop = store.startPolling(projectId, uploadSource);
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    stop();
    expect(seen).not.toContain(true);
    expect([store.error, store.documentsError]).toEqual([null, null]);
    expect(store.sources).toHaveLength(3);
    expect(store.documents[uploadSource]).toHaveLength(3);
  });
});
