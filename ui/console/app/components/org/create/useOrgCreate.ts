import type { ShownError } from '#base/lib/apperror/message';
import type { Org as OrgMessage } from '#gen/org/v1/org_pb';
import type { Org } from '~/stores/orgs';

import { create } from '@bufbuild/protobuf';
import { useValidation } from '#base/composables/useValidation';
import { CreateOwnOrgRequestSchema } from '#gen/auth/v1/auth_pb';
import { useOrgsStore } from '~/stores/orgs';
import { useUIStore } from '~/stores/ui';

export type OrgCreateCall = (req: { name: string, slug: string }) => Promise<{ org?: OrgMessage }>;

export interface OrgCreateOptions {
  create?: OrgCreateCall;
  landing?: (org: Org) => Promise<string>;
}

export function useOrgCreate(options: OrgCreateOptions = {}) {
  const name = ref('');
  const slug = ref('');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const orgs = useOrgsStore();
  const ui = useUIStore();
  const validation = useValidation(CreateOwnOrgRequestSchema, {
    fields: ['name', 'slug'],
    codes: { ORG002: 'slug', ORG003: 'slug', ORG004: 'name' },
  });
  const { $authClient: client } = useNuxtApp();

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
    if (!validation.check(create(CreateOwnOrgRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await (options.create ?? (r => client.createOwnOrg(r)))(req);
      const org = res.org;
      if (org) {
        const added = { id: org.id, slug: org.slug, name: org.name, role: 'owner' };
        orgs.add(added);
        ui.closeOrgCreate();
        name.value = '';
        slug.value = '';
        await navigateTo(options.landing ? await options.landing(added) : `/o/${org.slug}`);
      }
    } catch (e) {
      error.value = validation.fromServer(e, 'error.create_org');
    } finally {
      busy.value = false;
    }
  }

  return { open, name, slug, busy, error, fieldErrors: validation.errors, show: ui.openOrgCreate, hide, submit };
}
