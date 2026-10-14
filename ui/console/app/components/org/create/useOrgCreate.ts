import type { ShownError } from '~/lib/apperror/message';

import { create } from '@bufbuild/protobuf';
import { CreateOrgRequestSchema } from '~~/gen/org/v1/org_pb';
import { useValidation } from '~/composables/useValidation';
import { useOrgsStore } from '~/stores/orgs';
import { useUIStore } from '~/stores/ui';

export function useOrgCreate() {
  const name = ref('');
  const slug = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const orgs = useOrgsStore();
  const ui = useUIStore();
  const validation = useValidation(CreateOrgRequestSchema, {
    fields: ['name', 'slug'],
    codes: { ORG002: 'slug', ORG003: 'slug', ORG004: 'name' },
  });
  const { $orgClient: client } = useNuxtApp();

  const open = computed(() => ui.orgCreateOpen);

  function hide() {
    if (busy.value) {
      return;
    }
    ui.closeOrgCreate();
    error.value = null;
    validation.clear();
  }

  async function submit() {
    if (busy.value) {
      return;
    }
    const req = { name: name.value.trim(), slug: slug.value.trim() };
    if (!validation.check(create(CreateOrgRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.createOrg(req);
      const org = res.org;
      if (org) {
        orgs.add({ id: org.id, slug: org.slug, name: org.name });
        ui.closeOrgCreate();
        name.value = '';
        slug.value = '';
        await navigateTo(`/o/${org.slug}`);
      }
    } catch (e) {
      error.value = validation.fromServer(e, 'error.create_org');
    } finally {
      busy.value = false;
    }
  }

  return { open, name, slug, busy, error, fieldErrors: validation.errors, show: ui.openOrgCreate, hide, submit };
}
