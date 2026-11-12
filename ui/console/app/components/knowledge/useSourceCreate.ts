import type { ShownError } from '#base/lib/apperror/message';
import type { KnowledgeSource, SourceKind } from '~/stores/knowledge';
import { create } from '@bufbuild/protobuf';
import { useValidation } from '#base/composables/useValidation';
import { CreateUploadSourceRequestSchema, CreateURLSourceRequestSchema } from '#gen/knowledge/v1/knowledge_pb';
import { useKnowledgeStore } from '~/stores/knowledge';
import { defaultName, httpsAddress } from './knowledge';

// useSourceCreate holds the Add source form: a website by its https address, or a named upload source.
export function useSourceCreate(orgId: string, projectId: string) {
  const { t } = useI18n();
  const store = useKnowledgeStore();
  const open = ref(false);
  const kind = ref<SourceKind>('url');
  const name = ref('');
  const seedUrl = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);
  const urlError = ref('');
  const forUrl = useValidation(CreateURLSourceRequestSchema, { fields: ['name', 'seed_url'], codes: { KNW002: 'seed_url', KNW012: 'name' } });
  const forUpload = useValidation(CreateUploadSourceRequestSchema, { fields: ['name'], codes: { KNW012: 'name' } });
  const validation = computed(() => (kind.value === 'url' ? forUrl : forUpload));
  const errors = computed<Record<string, string>>(() => ({ ...validation.value.errors.value, ...(urlError.value ? { seed_url: urlError.value } : {}) }));

  function reset() {
    error.value = null;
    urlError.value = '';
    forUrl.clear();
    forUpload.clear();
  }

  function show() {
    kind.value = 'url';
    name.value = '';
    seedUrl.value = '';
    reset();
    open.value = true;
  }

  function hide() {
    if (!busy.value) {
      open.value = false;
    }
  }

  function request() {
    const url = seedUrl.value.trim();
    const typed = name.value.trim();
    return kind.value === 'url'
      ? { orgId, projectId, name: typed || defaultName(url), seedUrl: url }
      : { orgId, projectId, name: typed };
  }

  async function submit(): Promise<KnowledgeSource | null> {
    if (busy.value) {
      return null;
    }
    reset();
    const req = request();
    if (kind.value === 'url' && !httpsAddress(req.seedUrl ?? '')) {
      urlError.value = t('knowledge.url_invalid');
      return null;
    }
    const valid = kind.value === 'url' ? forUrl.check(create(CreateURLSourceRequestSchema, req)) : forUpload.check(create(CreateUploadSourceRequestSchema, req));
    if (!valid) {
      error.value = validation.value.formError.value;
      return null;
    }
    busy.value = true;
    try {
      const source = kind.value === 'url' ? await store.createUrlSource(orgId, projectId, req.name, req.seedUrl!) : await store.createUploadSource(orgId, projectId, req.name);
      open.value = false;
      return source;
    } catch (e) {
      error.value = validation.value.fromServer(e, 'error.create_knowledge_source');
      return null;
    } finally {
      busy.value = false;
    }
  }

  return { open, kind, name, seedUrl, busy, error, errors, show, hide, submit };
}
