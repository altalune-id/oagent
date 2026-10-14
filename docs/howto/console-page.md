# Add a console page

A page in the web console (`ui/console`): the Nuxt 4 single-page app served at `/console`. It is the only UI for signed-in people. It reads and writes through the Connect API (`/api`), so every call passes the permission and reach checks in `controlplane`.

## When to use

- A signed-in person views or changes something (orgs, projects, agents, keys, webhooks, members, profile).
- For a page that must work before sign-in, use [web-page.md](web-page.md).

## Plan

- [ ] The RPCs the page needs exist. If not, do [rpc.md](rpc.md) first and run `make generate` (it writes `ui/console/gen`).
- [ ] The URL: slugs only, under `app/pages/o/[org]/...`. API calls take public ids only ([public-ids.md](public-ids.md)).
- [ ] Which store holds the data (`app/stores/<plural>.ts`), and whether a new one is needed.
- [ ] Which form composables (`useXxx.ts`) and components it needs, and which components get a story.
- [ ] New i18n keys, in every locale. New error codes need [error-code.md](error-code.md).
- [ ] The nav entry (which sidebar).

## Build

1. **Client (new service only).** Add `createClient(XService, transport)` to [app/plugins/connect.client.ts](../../ui/console/app/plugins/connect.client.ts), its type to [app/types/nuxt.d.ts](../../ui/console/app/types/nuxt.d.ts) and its name to `ClientName` in [test/nuxt-globals.ts](../../ui/console/test/nuxt-globals.ts).
2. **Store.** `app/stores/<plural>.ts`, a Pinia setup store. Map the protobuf message to a plain interface with a `toX` function.

   ```ts
   export interface Thing {
     id: string;
     orgId: string;
     name: string;
   }

   export function toThing(t: ThingPB): Thing {
     return { id: t.id, orgId: t.orgId, name: t.name };
   }

   export const useThingsStore = defineStore('things', () => {
     const items = ref<Thing[]>([]);
     const loading = ref(false);
     const error = ref<ShownError | null>(null);
     const { $thingClient: client } = useNuxtApp();
     const errorMessage = useErrorMessage();

     async function load(orgId: string) {
       loading.value = true;
       error.value = null;
       try {
         const res = await client.listThings({ orgId });
         items.value = (res.things ?? []).map(toThing);
       } catch (e) {
         error.value = errorMessage(e, 'error.load_things');
       } finally {
         loading.value = false;
       }
     }

     return { items, loading, error, load };
   });
   ```

   Copy `ensureLoaded`, the `latest` counter (drop stale answers) and `add`/`replace`/`remove` from [app/stores/webhooks.ts](../../ui/console/app/stores/webhooks.ts) when the page needs them.

3. **Form composable.** `app/components/<area>/<action>/useThingCreate.ts`. Check the request against its proto rules before sending, and place server refusals under the same inputs.

   ```ts
   const validation = useValidation(CreateThingRequestSchema, {
     fields: ['name'],
     codes: { THG002: 'name' },
   });

   async function submit() {
     const req = { orgId, name: name.value.trim() };
     if (!validation.check(create(CreateThingRequestSchema, req))) {
       error.value = validation.formError.value;
       return;
     }
     busy.value = true;
     try {
       const res = await client.createThing(req);
       things.add(toThing(res.thing!));
     } catch (e) {
       error.value = validation.fromServer(e, 'error.create_thing');
     } finally {
       busy.value = false;
     }
   }
   ```

   Show `validation.errors` with `FieldError` and the form error with `ErrorText`. Other calls use `useErrorMessage()(e, 'error.<action>')`.

4. **Components.** `app/components/<area>/...`. `nuxt.config.ts` sets `components.dirs: []` and `imports.scan: false`: import every component, store and composable explicitly (Vue and Nuxt built-ins such as `ref`, `useI18n` and `useRoute` stay global). A component with a story is presentational: props in, events out, labels passed as props, relative imports, `computed` imported from `vue`, no `useI18n`, store or client (see `MemberList.vue`).
5. **Page.** `app/pages/o/[org]/.../<name>.vue`. Resolve the slugs from the route through the stores, throw a 404 when not found, set the title, and pass public ids down.

   ```vue
   <script setup lang="ts">
   import ThingList from '~/components/thing/list/ThingList.vue';
   import { useOrgsStore } from '~/stores/orgs';

   const route = useRoute();
   const { t } = useI18n();
   const orgs = useOrgsStore();
   const org = computed(() => orgs.bySlug(route.params.org as string));

   if (!org.value) {
     throw createError({ statusCode: 404, statusMessage: t('error.org_not_found'), fatal: true });
   }

   useHead({ title: () => `${t('things.title')} · oagent` });
   </script>
   ```

6. **Nav.** Add the item and its target to the matching sidebar: `AppSidebar.vue`, `SettingsSidebar.vue`, `ProjectSidebar.vue` or `AgentSidebar.vue`.
7. **i18n.** Add every key to every file in `ui/console/i18n/locales/` (`en-US.json`, `id-ID.json`). Keys are flat and dotted (`things.title`). Use `error.<action>` for fallbacks, `field.<proto_field>` for field names in validation messages and `apperror.<CODE>` / `apperror.<CODE>_<reason>` for error codes. `app/lib/apperror/catalog.test.ts` fails when the locales differ in keys or params, or an error code has no message.
8. **Stories.** `<Component>.stories.ts` next to a presentational component (`title: '<Area>/<Name>'`), one story per state: empty, filled, error, confirming.
9. **Tests.** `<file>.test.ts` next to every store, composable and helper. Fake clients with `setClient`, refusals with `connectError` ([test/nuxt-globals.ts](../../ui/console/test/nuxt-globals.ts)). Coverage counts `.ts` files only and fails below 90% lines, functions and statements, 85% branches, so keep logic out of `.vue` files.
10. **Embed.** `make ui-console` builds the SPA into `internal/web/console/public` (not committed) and writes a `.br` and `.gz` next to every text file over 1 KiB (`ui/scripts/compress.mts`). The Go handler serves those to browsers that accept them, caches the hashed `_nuxt/` files for a year and revalidates `index.html` by `ETag` (see `internal/boot/README.md`, Embedded assets). For development, `pnpm --filter @oagent/console dev` serves on port 8280 and proxies `/api` to `localhost:5150`.

## Style

ESLint runs the root [eslint.config.mjs](../../eslint.config.mjs) (`@antfu/eslint-config`) with the Nuxt rules on top ([ui/console/eslint.config.mjs](../../ui/console/eslint.config.mjs)): single quotes, semicolons, 2-space indent, `curly: all` (no one-line `if`), interface fields end with `;`, inline object types use `,`. Run `pnpm exec eslint --fix <files>`.

## Review

- [ ] Slugs appear only in URL paths; every API call sends public ids (`org.id`, never `org.slug`).
- [ ] Every component, store and composable is imported explicitly.
- [ ] Forms run `useValidation.check` before sending and `fromServer` on refusal.
- [ ] Every new key exists in every locale; no hard-coded user-facing text.
- [ ] Logic lives in `.ts` files with tests; coverage thresholds hold.
- [ ] Presentational components have stories and no store, client or i18n.
- [ ] Hiding a control the caller cannot use is fine, but the server decides.
- [ ] Gates pass: `make ui-test`, `pnpm -r typecheck`, `pnpm lint`, `pnpm --filter @oagent/console build-storybook`, `make ui`, `make check` (and `make generate` when protos changed).

## Reference code

- [app/pages/o/[org]/projects/[project]/webhooks/index.vue](../../ui/console/app/pages/o/[org]/projects/[project]/webhooks/index.vue): page shape
- [app/pages/o/[org]/settings/members.vue](../../ui/console/app/pages/o/[org]/settings/members.vue): page with labels for presentational components
- [app/stores/webhooks.ts](../../ui/console/app/stores/webhooks.ts): `useWebhooksStore`, `toWebhook`, `ensureLoaded`
- [app/components/webhook/create/useWebhookCreate.ts](../../ui/console/app/components/webhook/create/useWebhookCreate.ts): `useWebhookCreate`
- [app/components/webhook/create/WebhookCreateModal.vue](../../ui/console/app/components/webhook/create/WebhookCreateModal.vue)
- [app/components/member/MemberList.vue](../../ui/console/app/components/member/MemberList.vue) and [MemberList.stories.ts](../../ui/console/app/components/member/MemberList.stories.ts)
- [app/composables/useValidation.ts](../../ui/console/app/composables/useValidation.ts): `useValidation`, `ValidationOptions`
- [app/composables/useErrorMessage.ts](../../ui/console/app/composables/useErrorMessage.ts): `useErrorMessage`
- [app/components/project/ProjectSidebar.vue](../../ui/console/app/components/project/ProjectSidebar.vue): nav items
- [vitest.config.ts](../../ui/console/vitest.config.ts): coverage thresholds
