import type { UploadReject, UploadState, UploadTransport } from '#base/components/ui/upload';
import type { ShownError } from '#base/lib/apperror/message';
import type { EmbeddingModel as EmbeddingModelPB, KnowledgeDocument as KnowledgeDocumentPB, KnowledgeSource as KnowledgeSourcePB, RetrievalDefaults as RetrievalDefaultsPB } from '#gen/knowledge/v1/knowledge_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { UploadFailedError, xhrPut } from '#base/components/ui/upload';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { KnowledgeBackend, KnowledgeDocumentStatus, KnowledgeSourceKind, KnowledgeSourceStatus } from '#gen/knowledge/v1/knowledge_pb';

export type SourceKind = 'upload' | 'url';
export type SourceStatus = 'idle' | 'crawling' | 'failed';
export type DocumentStatus = 'uploading' | 'pending' | 'processing' | 'ready' | 'failed';
export type BackendName = 'pgvector';

export interface KnowledgeFailure {
  code: string;
  reason: string;
}

export interface AgentRef {
  id: string;
  name: string;
}

export interface KnowledgeSource {
  id: string;
  orgId: string;
  projectId: string;
  kind: SourceKind;
  backend: BackendName;
  name: string;
  seedUrl: string;
  status: SourceStatus;
  failure?: KnowledgeFailure;
  documents: number;
  ready: number;
  failed: number;
  usedBy: AgentRef[];
  updatedAt?: Date;
}

export interface KnowledgeDocument {
  id: string;
  sourceId: string;
  title: string;
  url: string;
  status: DocumentStatus;
  failure?: KnowledgeFailure;
  pages: number;
  bytes?: number;
  passages?: number;
  updatedAt?: Date;
}

export interface EmbeddingModel {
  providerId: string;
  model: string;
  dims: number;
  providerMissing?: boolean;
}

export interface KnowledgeHit {
  text: string;
  title: string;
  heading: string;
  page: number;
  url: string;
  score: number;
}

export interface RetrievalPreset {
  name: string;
  topK: number;
  minScore: number;
}

export interface RetrievalDefaults {
  topK: number;
  minScore: number;
  maxTokens: number;
  ranges: { topK: [number, number], minScore: [number, number], maxTokens: [number, number] };
  presets: RetrievalPreset[];
}

export interface UploadEntry {
  key: string;
  name: string;
  bytes: number;
  state: UploadState;
  progress: number;
  reason?: UploadReject['reason'];
  error?: string;
  documentId?: string;
}

export const pollIntervalMs = 2_000;

const extensions: Record<string, string[]> = {
  'application/pdf': ['.pdf'],
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': ['.docx'],
  'text/markdown': ['.md', '.markdown'],
  'text/plain': ['.txt'],
};

const kinds: Record<number, SourceKind> = { [KnowledgeSourceKind.UPLOAD]: 'upload', [KnowledgeSourceKind.URL]: 'url' };
const sourceStatuses: Record<number, SourceStatus> = { [KnowledgeSourceStatus.CRAWLING]: 'crawling', [KnowledgeSourceStatus.FAILED]: 'failed' };
const documentStatuses: Record<number, DocumentStatus> = {
  [KnowledgeDocumentStatus.UPLOADING]: 'uploading',
  [KnowledgeDocumentStatus.PENDING]: 'pending',
  [KnowledgeDocumentStatus.PROCESSING]: 'processing',
  [KnowledgeDocumentStatus.READY]: 'ready',
  [KnowledgeDocumentStatus.FAILED]: 'failed',
};
const backends: Record<number, BackendName> = { [KnowledgeBackend.PGVECTOR]: 'pgvector' };

// contentTypeOf names the allowed upload content type of a file by its extension, or '' when none fits.
export function contentTypeOf(name: string, allowed: string[]): string {
  const dot = name.lastIndexOf('.');
  const ext = dot < 0 ? '' : name.slice(dot).toLowerCase();
  return allowed.find(t => ext && extensions[t]?.includes(ext)) ?? '';
}

// acceptFor lists the file extensions of the allowed content types, for a file input's accept attribute.
export function acceptFor(allowed: string[]): string {
  return allowed.flatMap(t => extensions[t] ?? []).join(',');
}

// failureKeys lists the message keys of a failure, its reason first.
export function failureKeys(f: KnowledgeFailure): string[] {
  return f.reason ? [`apperror.${f.code}_${f.reason}`, `apperror.${f.code}`] : [`apperror.${f.code}`];
}

function toFailure(f?: { code: string, reason: string }): KnowledgeFailure | undefined {
  return f?.code ? { code: f.code, reason: f.reason } : undefined;
}

export function toSource(s: KnowledgeSourcePB): KnowledgeSource {
  return {
    id: s.id,
    orgId: s.orgId,
    projectId: s.projectId,
    kind: kinds[s.kind] ?? 'upload',
    backend: backends[s.backend] ?? 'pgvector',
    name: s.name,
    seedUrl: s.seedUrl,
    status: sourceStatuses[s.status] ?? 'idle',
    failure: toFailure(s.failure),
    documents: s.documentCount,
    ready: s.readyCount,
    failed: s.failedCount,
    usedBy: s.usedBy.map(u => ({ id: u.agentId, name: u.agentName })),
    updatedAt: s.updatedAt && timestampDate(s.updatedAt),
  };
}

export function toDocument(d: KnowledgeDocumentPB): KnowledgeDocument {
  return {
    id: d.id,
    sourceId: d.sourceId,
    title: d.title,
    url: d.url,
    status: documentStatuses[d.status] ?? 'pending',
    failure: toFailure(d.failure),
    pages: d.pageCount,
    bytes: d.sizeBytes > 0n ? Number(d.sizeBytes) : undefined,
    passages: d.chunkCount > 0 ? d.chunkCount : undefined,
    updatedAt: d.updatedAt && timestampDate(d.updatedAt),
  };
}

function toRetrievalDefaults(d?: RetrievalDefaultsPB): RetrievalDefaults | null {
  return d
    ? {
        topK: d.topK,
        minScore: d.minScore,
        maxTokens: d.maxTokens,
        ranges: { topK: [d.minTopK, d.maxTopK], minScore: [d.minMinScore, d.maxMinScore], maxTokens: [d.minMaxTokens, d.maxMaxTokens] },
        presets: d.presets.map(p => ({ name: p.name, topK: p.topK, minScore: p.minScore })),
      }
    : null;
}

function toEmbedding(e?: EmbeddingModelPB): EmbeddingModel | null {
  return e ? { providerId: e.providerId, model: e.model, dims: e.dims, providerMissing: e.providerMissing } : null;
}

function ingesting(s: KnowledgeSource, docs?: KnowledgeDocument[]): boolean {
  return docs
    ? docs.some(d => d.status === 'pending' || d.status === 'processing') || docs.filter(d => d.status === 'ready').length !== s.ready
    : s.documents > s.ready + s.failed;
}

// needsPolling reports whether a crawl runs or documents still wait for ingestion that can happen now; a browser upload in flight doesn't count.
export function needsPolling(sources: KnowledgeSource[], embedding: EmbeddingModel | null, documents: Record<string, KnowledgeDocument[]> = {}): boolean {
  return sources.some(s => s.status === 'crawling' || (!!embedding && ingesting(s, documents[s.id])));
}

export const useKnowledgeStore = defineStore('knowledge', () => {
  const sources = ref<KnowledgeSource[]>([]);
  const storageConfigured = ref(false);
  const maxUploadBytes = ref(0);
  const allowedContentTypes = ref<string[]>([]);
  const retrievalDefaults = ref<RetrievalDefaults | null>(null);
  const uploadAccept = computed(() => acceptFor(allowedContentTypes.value));
  const enabledBackends = ref<BackendName[]>([]);
  const embedding = ref<EmbeddingModel | null>(null);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const documents = ref<Record<string, KnowledgeDocument[]>>({});
  const documentsLoading = ref(false);
  const documentsError = ref<ShownError | null>(null);
  const uploads = ref<Record<string, UploadEntry[]>>({});

  const { $knowledgeClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const on = computed(() => enabledBackends.value.length > 0);
  let loadedFor = '';
  let latest = 0;
  let latestLoud = 0;
  let latestDocuments = 0;
  let latestDocumentsLoud = 0;
  let seq = 0;

  function byId(id: string) {
    return sources.value.find(s => s.id === id);
  }

  function put(source: KnowledgeSource) {
    sources.value = byId(source.id) ? sources.value.map(s => (s.id === source.id ? source : s)) : [source, ...sources.value];
  }

  async function load(projectId: string, quiet = false) {
    const mine = ++latest;
    const loud = quiet ? 0 : ++latestLoud;
    if (!quiet) {
      loading.value = true;
      error.value = null;
    }
    try {
      const res = await client.listKnowledgeSources({ projectId });
      if (mine !== latest) {
        return;
      }
      error.value = null;
      sources.value = (res.sources ?? []).map(toSource);
      storageConfigured.value = res.storageConfigured;
      maxUploadBytes.value = Number(res.maxUploadBytes ?? 0n);
      allowedContentTypes.value = [...(res.allowedContentTypes ?? [])];
      retrievalDefaults.value = toRetrievalDefaults(res.retrievalDefaults);
      enabledBackends.value = (res.enabledBackends ?? []).map(b => backends[b]).filter((b): b is BackendName => !!b);
      embedding.value = toEmbedding(res.embeddingModel);
      loadedFor = projectId;
    } catch (e) {
      if (mine === latest && !quiet) {
        error.value = errorMessage(e, 'error.load_knowledge');
      }
    } finally {
      if (loud && loud === latestLoud) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  async function createUrlSource(orgId: string, projectId: string, name: string, seedUrl: string) {
    const res = await client.createURLSource({ orgId, projectId, name, seedUrl });
    const source = toSource(res.source!);
    put(source);
    return source;
  }

  async function createUploadSource(orgId: string, projectId: string, name: string) {
    const res = await client.createUploadSource({ orgId, projectId, name });
    const source = toSource(res.source!);
    put(source);
    return source;
  }

  async function refresh(id: string) {
    const res = await client.refreshKnowledgeSource({ id });
    put(toSource(res.source!));
  }

  async function removeSource(id: string) {
    await client.deleteKnowledgeSource({ id });
    sources.value = sources.value.filter(s => s.id !== id);
    const { [id]: _, ...rest } = documents.value;
    documents.value = rest;
  }

  async function setEmbedding(projectId: string, providerId: string, model: string) {
    const res = await client.setEmbeddingModel({ projectId, providerId, model });
    embedding.value = toEmbedding(res.embeddingModel);
  }

  async function search(sourceId: string, query: string): Promise<KnowledgeHit[]> {
    const res = await client.testSearch({ sourceId, query: query.trim() });
    return (res.hits ?? []).map(h => ({ text: h.text, title: h.title, heading: h.heading, page: h.page, url: h.url, score: h.score }));
  }

  async function loadDocuments(sourceId: string, quiet = false) {
    const mine = ++latestDocuments;
    const loud = quiet ? 0 : ++latestDocumentsLoud;
    if (!quiet) {
      documentsLoading.value = true;
      documentsError.value = null;
    }
    try {
      const res = await client.listKnowledgeDocuments({ sourceId });
      if (mine === latestDocuments) {
        documentsError.value = null;
        documents.value = { ...documents.value, [sourceId]: (res.documents ?? []).map(toDocument) };
      }
    } catch (e) {
      if (mine === latestDocuments && !quiet) {
        documentsError.value = errorMessage(e, 'error.load_knowledge_documents');
      }
    } finally {
      if (loud && loud === latestDocumentsLoud) {
        documentsLoading.value = false;
      }
    }
  }

  function dropDocument(sourceId: string, id: string) {
    documents.value = { ...documents.value, [sourceId]: (documents.value[sourceId] ?? []).filter(d => d.id !== id) };
  }

  function putDocument(doc: KnowledgeDocument) {
    const list = documents.value[doc.sourceId] ?? [];
    documents.value = { ...documents.value, [doc.sourceId]: list.some(d => d.id === doc.id) ? list.map(d => (d.id === doc.id ? doc : d)) : [doc, ...list] };
  }

  async function removeDocument(sourceId: string, id: string) {
    await client.deleteKnowledgeDocument({ id });
    dropDocument(sourceId, id);
  }

  function entries(sourceId: string, files: Omit<UploadEntry, 'key' | 'progress'>[]): UploadEntry[] {
    const added = files.map(f => ({ key: `u${++seq}`, progress: 0, ...f }));
    uploads.value = { ...uploads.value, [sourceId]: [...(uploads.value[sourceId] ?? []), ...added] };
    return uploads.value[sourceId]!.slice(-added.length);
  }

  // refuse lists files the upload control turned down, so their reason shows in the queue.
  function refuse(sourceId: string, rejects: UploadReject[]) {
    entries(sourceId, rejects.map(r => ({ name: r.file.name, bytes: r.file.size, state: 'failed', reason: r.reason })));
  }

  // abandon deletes a document whose file never reached storage, best effort; the storage sweep fails it otherwise.
  function abandon(sourceId: string, id: string) {
    dropDocument(sourceId, id);
    client.deleteKnowledgeDocument({ id }).catch(() => {});
  }

  async function sendOne(sourceId: string, file: File, entry: UploadEntry, transport: UploadTransport) {
    entry.state = 'uploading';
    try {
      const res = await client.beginUpload({ sourceId, filename: file.name, contentType: contentTypeOf(file.name, allowedContentTypes.value), sizeBytes: BigInt(file.size) });
      const id = res.document!.id;
      putDocument(toDocument(res.document!));
      entry.documentId = id;
      try {
        if (res.upload?.method !== 'PUT') {
          throw new UploadFailedError(0);
        }
        await transport(res.upload.url, file, { ...res.upload.headers }, (f) => {
          entry.progress = f;
        });
      } catch (e) {
        abandon(sourceId, id);
        throw e;
      }
      const done = await client.completeUpload({ documentId: res.document!.id });
      putDocument(toDocument(done.document!));
      entry.progress = 1;
      entry.state = 'done';
    } catch (e) {
      entry.state = 'failed';
      entry.error = errorMessage(e, 'knowledge.upload_put_failed').message;
    }
  }

  // upload sends files one at a time: presign, then the browser PUTs the file to storage, then complete.
  async function upload(sourceId: string, files: File[], transport: UploadTransport = xhrPut) {
    const queued = entries(sourceId, files.map(f => ({ name: f.name, bytes: f.size, state: 'queued' })));
    for (const [i, file] of files.entries()) {
      await sendOne(sourceId, file, queued[i]!, transport);
    }
  }

  // startPolling quietly reloads the sources, and one source's documents, until nothing is crawling or waiting; only its caller stops it.
  function startPolling(projectId: string, sourceId = ''): () => void {
    const timer = setInterval(async () => {
      await load(projectId, true);
      if (sourceId) {
        await loadDocuments(sourceId, true);
      }
      if (!needsPolling(sources.value, embedding.value, documents.value)) {
        clearInterval(timer);
      }
    }, pollIntervalMs);
    return () => clearInterval(timer);
  }

  return {
    sources,
    storageConfigured,
    maxUploadBytes,
    allowedContentTypes,
    uploadAccept,
    retrievalDefaults,
    enabledBackends,
    embedding,
    on,
    loading,
    error,
    documents,
    documentsLoading,
    documentsError,
    uploads,
    byId,
    load,
    ensureLoaded,
    createUrlSource,
    createUploadSource,
    refresh,
    removeSource,
    setEmbedding,
    search,
    loadDocuments,
    removeDocument,
    refuse,
    upload,
    startPolling,
  };
});
