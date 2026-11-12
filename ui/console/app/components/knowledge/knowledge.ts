import type { StatusTone } from '#base/components/ui/types';
import type { EmbeddingModel, KnowledgeDocument, KnowledgeFailure, KnowledgeSource } from '~/stores/knowledge';
import type { ModelProvider } from '~/stores/modelProviders';
import { ModelProviderKind } from '#gen/modelprovider/v1/modelprovider_pb';
import { failureKeys } from '~/stores/knowledge';

export interface StateLabel {
  tone: StatusTone;
  key: string;
  params?: Record<string, number>;
}

const nameMax = 200;

// httpsAddress reports whether a seed URL is a full https address with a host.
export function httpsAddress(url: string): boolean {
  try {
    const u = new URL(url.trim());
    return u.protocol === 'https:' && !!u.host;
  } catch {
    return false;
  }
}

// defaultName names a website source from its URL's host and path, or '' when the URL doesn't parse.
export function defaultName(url: string): string {
  try {
    const u = new URL(url.trim());
    return `${u.host}${u.pathname.replace(/\/$/, '')}`.slice(0, nameMax);
  } catch {
    return '';
  }
}

// sourceState is a source's status as people read it: a crawl, a failure, or how far its documents are.
export function sourceState(s: KnowledgeSource): StateLabel {
  if (s.status === 'crawling') {
    return { tone: 'info', key: 'knowledge.source_crawling' };
  }
  if (s.status === 'failed' || (s.documents > 0 && s.failed === s.documents)) {
    return { tone: 'error', key: 'knowledge.source_failed' };
  }
  if (s.ready > 0) {
    return s.failed > 0 ? { tone: 'warning', key: 'knowledge.source_ready_failed', params: { n: s.failed } } : { tone: 'success', key: 'knowledge.source_ready' };
  }
  return s.documents > 0 ? { tone: 'info', key: 'knowledge.source_reading' } : { tone: 'neutral', key: 'knowledge.source_empty' };
}

const documentStates: Record<KnowledgeDocument['status'], StateLabel> = {
  uploading: { tone: 'info', key: 'knowledge.doc_uploading' },
  pending: { tone: 'neutral', key: 'knowledge.doc_pending' },
  processing: { tone: 'info', key: 'knowledge.doc_processing' },
  ready: { tone: 'success', key: 'knowledge.doc_ready' },
  failed: { tone: 'error', key: 'knowledge.doc_failed' },
};

// documentState is a document's status; one waiting while the project has no embedding model is a warning.
export function documentState(d: KnowledgeDocument, hasModel = true): StateLabel {
  return d.status === 'pending' && (!hasModel || d.failure?.code === 'KNW011') ? { tone: 'warning', key: 'knowledge.doc_waiting_model' } : documentStates[d.status];
}

export interface EmbeddingChoice {
  providerId: string;
  providerName: string;
  model: string;
  priced: boolean;
}

// embeddingChoices lists the models marked for embeddings on providers with an OpenAI-compatible embeddings API.
export function embeddingChoices(providers: ModelProvider[]): EmbeddingChoice[] {
  return providers
    .filter(p => p.kind === ModelProviderKind.OPENAI_COMPATIBLE)
    .flatMap(p => p.models.filter(m => m.embedding).map(m => ({ providerId: p.id, providerName: p.name, model: m.id, priced: !!m.prices })));
}

// reembeds reports whether picking a model replaces one already in use while sources exist, so every document is embedded again.
export function reembeds(current: EmbeddingModel | null, next: { providerId: string, model: string }, sources: number): boolean {
  return !!current && sources > 0 && (current.providerId !== next.providerId || current.model !== next.model);
}

// failureText words a source or document failure, its reason first; console texts stand in for catalog texts that need params a failure doesn't carry.
export function failureText(f: KnowledgeFailure, t: (key: string) => string, te: (key: string) => boolean): string {
  const key = [`knowledge.fail_${f.code}_${f.reason}`, ...failureKeys(f)].find(te);
  return t(key ?? 'knowledge.failed_unknown');
}
