import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { vi } from 'vitest';
import { EmbeddingModelSchema, KnowledgeBackend, KnowledgeDocumentSchema, KnowledgeDocumentStatus, KnowledgeSourceKind, KnowledgeSourceSchema, KnowledgeSourceStatus } from '#gen/knowledge/v1/knowledge_pb';
import { setClient } from '#test/nuxt-globals';

export const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
export const projectId = 'x6MH9WPWjktMwCBKHy4pF';
export const updated = timestampFromDate(new Date('2026-11-10T08:00:00Z'));

export function sourcePBs() {
  return [
    create(KnowledgeSourceSchema, { id: 'Ks1Upl0adQ8sWx3YcLm7D', orgId, projectId, kind: KnowledgeSourceKind.UPLOAD, backend: KnowledgeBackend.PGVECTOR, name: 'Refund policy', status: KnowledgeSourceStatus.IDLE, documentCount: 3, readyCount: 2, failedCount: 1, usedBy: [{ agentId: 'Ag7Rf2Tq9Lm4Xw8Pz3Kc1', agentName: 'Refund Triage' }], updatedAt: updated }),
    create(KnowledgeSourceSchema, { id: 'Ks2Ur1CrawlTq9Lm4Xw8P', orgId, projectId, kind: KnowledgeSourceKind.URL, backend: KnowledgeBackend.PGVECTOR, name: 'help.example.com/docs', seedUrl: 'https://help.example.com/docs', status: KnowledgeSourceStatus.CRAWLING, documentCount: 4, readyCount: 0, updatedAt: updated }),
    create(KnowledgeSourceSchema, { id: 'Ks3Fa1ledRp5Nw2Qk7Vb4', orgId, projectId, kind: KnowledgeSourceKind.URL, backend: KnowledgeBackend.PGVECTOR, name: 'shop.example.com', seedUrl: 'https://shop.example.com', status: KnowledgeSourceStatus.FAILED, failure: { code: 'KNW014', reason: 'robots_disallowed' }, updatedAt: updated }),
  ];
}

export const docPB = (init: { id: string, sourceId: string, title: string, status: KnowledgeDocumentStatus }) => create(KnowledgeDocumentSchema, init);

export function documentPBs() {
  return [
    create(KnowledgeDocumentSchema, { id: 'Kd1Ready0Tq9Lm4Xw8Pz3', sourceId: 'Ks1Upl0adQ8sWx3YcLm7D', title: 'refund-policy.pdf', status: KnowledgeDocumentStatus.READY, pageCount: 3, updatedAt: updated }),
    create(KnowledgeDocumentSchema, { id: 'Kd2Wait0Rp5Nw2Qk7Vb4x', sourceId: 'Ks1Upl0adQ8sWx3YcLm7D', title: 'faq.md', status: KnowledgeDocumentStatus.PENDING, failure: { code: 'KNW011' }, updatedAt: updated }),
    create(KnowledgeDocumentSchema, { id: 'Kd3Fail0Lm4Xw8Pz3Kc1q', sourceId: 'Ks1Upl0adQ8sWx3YcLm7D', title: 'scan.pdf', status: KnowledgeDocumentStatus.FAILED, failure: { code: 'KNW005' }, updatedAt: updated }),
  ];
}

export const allowedTypes = ['application/pdf', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', 'text/markdown', 'text/plain'];

export const retrievalDefaultsPB = { topK: 6, minScore: 0.25, maxTokens: 3000, minTopK: 1, maxTopK: 10, minMinScore: 0, maxMinScore: 1, minMaxTokens: 500, maxMaxTokens: 8000, presets: [{ name: 'precise', topK: 3, minScore: 0.35 }, { name: 'balanced', topK: 6, minScore: 0.25 }, { name: 'broad', topK: 8, minScore: 0.1 }] };

export const embeddingPB = create(EmbeddingModelSchema, { providerId: 'Pv6tQm2ZxR8wKp3VbNc4K', model: 'openai/text-embedding-3-small', dims: 1536 });

// fakeKnowledgeClient installs a KnowledgeService fake with the fixtures above.
export function fakeKnowledgeClient(over: { storage?: boolean, backends?: KnowledgeBackend[], embedding?: boolean } = {}) {
  const client = {
    listKnowledgeSources: vi.fn().mockResolvedValue({ sources: sourcePBs(), maxUploadBytes: 10_485_760n, allowedContentTypes: allowedTypes, retrievalDefaults: retrievalDefaultsPB, storageConfigured: over.storage ?? true, enabledBackends: over.backends ?? [KnowledgeBackend.PGVECTOR], embeddingModel: over.embedding === false ? undefined : embeddingPB }),
    getKnowledgeSource: vi.fn(async ({ id }: { id: string }) => ({ source: sourcePBs().find(s => s.id === id) })),
    createURLSource: vi.fn(async (req: { name: string, seedUrl: string }) => ({ source: create(KnowledgeSourceSchema, { ...req, id: 'Ks9NewUrlQ8sWx3YcLm7D', kind: KnowledgeSourceKind.URL, backend: KnowledgeBackend.PGVECTOR, status: KnowledgeSourceStatus.CRAWLING, updatedAt: updated }) })),
    createUploadSource: vi.fn(async (req: { name: string }) => ({ source: create(KnowledgeSourceSchema, { ...req, id: 'Ks8NewUplQ8sWx3YcLm7D', kind: KnowledgeSourceKind.UPLOAD, backend: KnowledgeBackend.PGVECTOR, status: KnowledgeSourceStatus.IDLE, updatedAt: updated }) })),
    refreshKnowledgeSource: vi.fn(async ({ id }: { id: string }) => ({ source: create(KnowledgeSourceSchema, { ...sourcePBs().find(s => s.id === id)!, status: KnowledgeSourceStatus.CRAWLING, failure: undefined as never }) })),
    deleteKnowledgeSource: vi.fn().mockResolvedValue({}),
    listKnowledgeDocuments: vi.fn().mockResolvedValue({ documents: documentPBs() }),
    deleteKnowledgeDocument: vi.fn().mockResolvedValue({}),
    beginUpload: vi.fn(async (req: { sourceId: string, filename: string }) => ({
      document: create(KnowledgeDocumentSchema, { id: `Kd-${req.filename}`, sourceId: req.sourceId, title: req.filename, status: KnowledgeDocumentStatus.UPLOADING }),
      upload: { url: `https://bucket.example.com/${req.filename}?sig=1`, method: 'PUT', headers: { 'Content-Type': 'application/pdf' } },
    })),
    completeUpload: vi.fn(async ({ documentId }: { documentId: string }) => ({ document: create(KnowledgeDocumentSchema, { id: documentId, sourceId: 'Ks1Upl0adQ8sWx3YcLm7D', title: documentId.slice(3), status: KnowledgeDocumentStatus.PENDING }) })),
    setEmbeddingModel: vi.fn(async (req: { providerId: string, model: string }) => ({ embeddingModel: create(EmbeddingModelSchema, { ...req, dims: 1024 }) })),
    testSearch: vi.fn().mockResolvedValue({ hits: [{ text: 'Refunds take 5 working days.', title: 'refund-policy.pdf', heading: 'Timing', page: 2, url: '', score: 0.8312 }] }),
  };
  setClient('$knowledgeClient', client);
  return client;
}
