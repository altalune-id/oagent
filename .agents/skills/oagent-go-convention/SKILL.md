---
name: oagent-go-convention
description: Use when planning or building any change in oagent (altalune.id/oagent) — a new or changed RPC, MCP tool, CLI command, domain module, service or store method, migration, config key, error code, worker, webhook event, rate limit, pre-sign-in page or console page — or when asked "how do I add X here", where a file goes, or what the convention is for public ids, the permission table, protovalidate rules, the unit of work or the boot wiring.
license: Proprietary
metadata:
  pairs-with: oagent-go-review (checks what this skill builds)
---

# Building oagent changes

oagent's conventions are written down once, in the repo. This skill tells you which document to read, in which order,
and runs the gates. It does not repeat the rules: when this skill and the repo disagree, the repo wins.

## 1. Read the rules

1. `AGENTS.md` (repo root): rules, gates and git policy. Read it all; it is short.
2. `docs/architecture/README.md`: the layers and the import direction.
3. The recipe(s) for your change in `docs/howto/` (table below). Follow its **Plan**, **Build** and **Review** sections in order.

## 2. Pick the recipes

Most changes need more than one recipe. Read every row that applies.

| The change | Recipes |
| --- | --- |
| new domain package | `module.md`, `store-method.md`, `service-method.md`, `migration.md` |
| new or changed API call | `rpc.md`, `service-method.md`, `error-code.md`, then `mcp-tool.md`, `cli-command.md`, `console-page.md` for each surface that exposes it |
| any identifier a user or client sees | `public-ids.md` |
| database read or write | `store-method.md` (and `migration.md` if the schema changes) |
| new table, column, index or constraint | `migration.md` (updates `schema/README.md` in the same commit) |
| user-facing failure | `error-code.md` |
| config key | `config-key.md` |
| background job, consumer or sweep | `worker.md` (updates `internal/boot/README.md`) |
| event delivered to webhooks | `webhook-event.md` |
| throttling an action | `rate-limit.md` |
| page shown before sign-in | `web-page.md` |
| anything a signed-in person sees | `console-page.md` |

## 3. Rules that are expensive to get wrong

These are the ones reviews keep finding. The recipes have the detail.

- **Public ids on every surface.** API, MCP, CLI, console calls, webhook payloads and errors carry `public_id`. UUIDs never leave the server; slugs only in URL paths.
- **Signed-in surfaces call `controlplane`.** Connect, MCP and signed-in pages never call a service directly. Only pre-sign-in pages, operator commands that boot the `App`, and workers do.
- **Permission table.** Every RPC has an entry in `internal/permission`; a missing entry fails `TestEveryRPCHasAPermission`. Reach tests prove a principal cannot touch another org's or project's rows.
- **Validation lives in the proto.** protovalidate rules on every request field (shared `public_id`/`name`/`email` rules). The Connect interceptor, the MCP tools and the console forms all use the same rules.
- **Unit of work.** A write and its event or outbox row commit together through `db.Transactor` (see `service-method.md`).
- **Layering.** Domains import only `internal/kernel/*`, root packages, other domains' ports and `internal/web/layout`. depguard `domain-boundary` checks every `internal/*` package except the shells; the rest is review.
- **Features pick a driver; connections are root libraries.** A feature lives in `internal/kernel/<feature>`, names its driver in its own block (`db.driver`, `rateLimit.driver`, `queue.driver`) and takes a driver-agnostic interface (`ratelimit.Limiter`, `queue.Queue`) or, for the database, `db.Pool` and `db.Transactor`. A feature with several backends keeps each in a sub-package (`queue/nats`, `ratelimit/memory`, `ratelimit/redis`); one that must commit with domain data (`outbox`, `mailqueue`, `session`) keeps its Postgres store in-package. The connection is its own block (`postgres.*`, `redis.*`, `nats.*`) and a root package (`pg`, `redis`, `nats`) that opens, health-checks and closes it and returns the real client; boot opens it only when `<Backend>Features()` lists a user. Kernel packages and connections never import `config`: one that needs settings has its own `Config` in `config.go`, mapped by boot (depguard `config-boundary`). Only the connection, its drivers, boot and test helpers import pgx, go-redis or NATS (`pgx-boundary`, `redis-boundary`, `nats-boundary`); domains use `db.IsUniqueViolation`, `db.IsForeignKeyViolation` and `db.ViolatedConstraint`, and never import `pg`, `nats` or `redis`. On a shared NATS server each app has its own account; names are not prefixed.
- **Error codes are append-only** and translated in every locale of the `ui/base` layer.
- **Live-database migrations.** The database is never reset: each migration applies to existing rows while the old app still runs (a new `NOT NULL` column gets a `DEFAULT`, dropped in a later release only once every insert writes it), with no data guards or SQL id generators.
- **READMEs move with the code.** `schema/README.md` for schema changes, `internal/boot/README.md` for wiring changes, in the same commit.
- **Comments.** Default none; follow the `alt-go:alt-comment-discipline` skill. `make comment-check` must stay at 0.
- **Style.** Go: gofmt + gci import groups. TS/Vue: ESLint with semicolons; interface fields end with `;`, inline object types use `,`; never a one-line `if`.

## 4. Build

- Copy the shape of the reference code each recipe links. Prefer an existing helper over a new one.
- TDD: write the failing test first (reach, permission, validation, public-id tests included), then the code.
- Go: follow the `go:go` and `alt-go:alt-go-concurrency` skills, and `alt-go:alt-go-proverb` for judgment calls.
- UI: follow `antfu-frontend:vue`, `antfu-frontend:pinia`, `antfu-frontend:nuxt`, `antfu-frontend:vitest`, `eslint-config` and the UI section below.

## 5. Verify

```sh
.agents/skills/oagent-go-convention/scripts/verify.sh                # regenerate, then every unit gate
.agents/skills/oagent-go-convention/scripts/verify.sh --integration  # plus make cover (Postgres via testcontainers)
.agents/skills/oagent-go-convention/scripts/verify.sh --ui           # plus typecheck, UI tests, build, Storybook
```

Then open what you changed: a console page in the browser, a CLI command in a terminal, an MCP tool from a client. A green build does not prove a surface works.

## Never

- Commit, stage or push. The user commits.
- Edit generated code (`gen/`, `ui/base/gen/`, `*_templ.go`) by hand.
- Weaken a linter, a coverage floor or a test to get green.

## UI

**daisyUI**

- daisyUI 5 is installed in three packages, pinned in the `pnpm-workspace.yaml` catalog: the console (`ui/console` on the `ui/base` layer, Tailwind 4 through `@tailwindcss/vite`, CSS in `ui/base/app/assets/css/main.css`), the sign-in pages (`ui/web/app.css`, which scans `internal/**/*.templ`; run `make web-css` after adding classes) and the MCP view (`ui/mcp/src/style.css`: `themes: false`, `@import` of `ui/theme.css`, and `@source` on `ui/mcp/src` and `ui/base/app/components/ui`).
- Built-in themes are off (`@plugin "daisyui" { themes: false; }`). Every theme is a light and dark pair in `ui/theme.css` (`oagent-light`/`oagent-dark`, `bumblebee-light`/`bumblebee-dark` and so on), registered as a family in `ui/base/app/components/layout/useTheme.ts`. The console sets `data-theme` on `<html>`; the sign-in pages set no `data-theme`, so they follow the system: `oagent-light`, or `oagent-dark` (its `prefersdark: true`) in dark mode. Keep `prefersdark` on that one theme only. To add a theme: add both modes with `@plugin "daisyui/theme"`, add the dark one to the `dark` custom variant at the top of `ui/theme.css`, and register the family. Do not change themes through the plugin options.
- Use semantic colors only (`bg-base-100`, `text-base-content/70`, `btn-primary`, `border-base-300`), so every theme works. `dark:` is mapped to the `*-dark` themes, but do not pair it with daisyUI colors: they already follow the theme.
- In the console, reuse `ui/base/app/components/ui` (`AppButton`, `AppModal`, `ConfirmModal`, `AppDropdown`, `LoadError`, `LoadingStatus`, `NavbarShell`, `SidebarNav`, `AppShell`, `PageHeader`, `Section`, `DataTable`, `EmptyState`, `FormField`, `KeyValue`, `SecretField`, `ToastHost` + `useToast`, `FieldError` and others) before writing raw daisyUI markup, and follow docs/howto/console-page.md. For templ pages, follow docs/howto/web-page.md.
- Muted text is `text-base-content/70`; lower opacities fail WCAG AA in some themes (a guard in `ui/base/test/markup.test.ts` checks it, and checks every theme's contrast).
- Dropdowns: the console uses only the popover method, through `AppDropdown` (anchored with `anchor-name`/`position-anchor`); templ pages (nonce CSP, no inline style) use only `<details class="dropdown">`. Never the CSS-focus method (`tabindex` + `dropdown-content`).
- The binary embeds its assets: do not add CDN links, and do not use external placeholder images in product UI.

**Vue**

- Vue 3.5 (from the workspace `catalog:`) in `ui/console` (Nuxt 4 SPA, `ssr: false`) and `ui/mcp` (Vite single-file app). No SSR anywhere.
- Vue APIs (`ref`, `computed`, `watch`, ...) are auto-imported in both apps; shared `components/ui/*` files import them from `'vue'` explicitly. Follow the neighboring files.
- Components and own composables are imported explicitly (`components.dirs: []`, `imports.scan: false` in `nuxt.config.ts`).
- Styling is Tailwind CSS 4 + daisyUI 5 classes in the template; not UnoCSS. Visible text comes from `@nuxtjs/i18n` (`useI18n().t`), never hard-coded.
- Shared building blocks live in the `ui/base` Nuxt layer (`ui/base/app/components/ui/`, imported as `#base/components/ui/...`) and have Storybook 10 stories; reuse them before writing new markup (`ConfirmModal` + `useConfirm`, `LoadError`, `LoadingStatus`, `AppDropdown`, `useDates`).
- Show and pass public ids, never UUIDs; slugs only in route paths.
- Upstream examples use no semicolons; write new code in oagent's lint style (see `eslint-config`).

**Pinia**

- `ui/console` uses Pinia 4 through `@pinia/nuxt`: no server state, no hydration.
- Write setup stores only: `defineStore('orgs', () => { ... })` with `ref`/`computed` and plain functions; one store per file in `app/stores/`.
- `defineStore`, `ref` and `computed` are auto-imported; stores, components and own composables are imported explicitly.
- Stores call the Connect clients from `useNuxtApp()` (`$orgClient` and friends) and expose `items`, `loading`, `loaded`, `error` plus actions such as `load()` and `ensureLoaded()`.
- Errors shown to users go through `useErrorMessage()`; ids held in state are public ids.
- Test each store next to it (`orgs.test.ts`) with `setActivePinia(createPinia())` and mocked clients from `#test/nuxt-globals` (`ui/base/test/nuxt-globals.ts`); `@pinia/testing` is not installed.

**Vite**

- Vite 8 (Rolldown + Oxc) comes from the pnpm 10 workspace `catalog:`; reference it as `"vite": "catalog:"`, never a pinned copy per package.
- `ui/mcp` is a Vite + Vue single-file SPA (`vite-plugin-singlefile`, `unplugin-auto-import` for Vue APIs, `@tailwindcss/vite`, and `iconSubset`, which inlines only the lucide icons its sources name); it imports `ui/base` through `baseAliases`, and `pnpm ui:mcp` builds it.
- `ui/web` uses Vite only to emit one ES module (`build.lib`, one entry) into `internal/web/static`; its CSS comes from the Tailwind CLI, not Vite.
- `ui/console` changes its Vite settings through `vite:` in `nuxt.config.ts`, not a `vite.config.ts`.
- No SSR and no published libraries: every build is client code embedded in the Go binary. See docs/howto/README.md for the build and embed workflow.

**Vitest**

- Vitest 5 and `@vitest/coverage-v8` come from the workspace `catalog:`; every package extends a preset from `ui/vitest-config` (`node` for pure TS, `browser` for happy-dom, `vue` for mounted components: the Vue plugin, `Icon` and `NuxtLink` stubs, auto-unmount).
- `globals: false`: import `describe`, `it`, `expect`, `vi`, `beforeEach` from `'vitest'` in every file.
- Write `it(...)` inside a `describe(...)` block; do not switch to top-level `test(...)`.
- Tests sit next to the code (`app/stores/orgs.test.ts`); `#test/nuxt-globals` stands in for Nuxt auto-imports and Connect clients (`setClient`, `connectError`).
- `ui/console` and `ui/base` run `vitest run --coverage` with thresholds (`ui/base/vitest.app.ts`) (lines/statements/functions 90, branches 85); a drop below fails the run.
- Mount components with `@vue/test-utils` in a file that starts with `// @vitest-environment happy-dom`; use `mountPage` (`#test/mount-page`) for a page or an async setup. Page tests live in `ui/<app>/test/`.
- Fixture ids are public ids. Run all UI tests with `pnpm ui:test`.

**Nuxt**

- `ui/console` is a Nuxt 4 SPA (`nuxt@^4.5`): `ssr: false`, built with `nuxt generate` and embedded into the Go binary (`make ui-console`). There is no Nuxt server, no `server/` directory and no server routes; the Go binary is the backend.
- The `srcDir` is `app/` (`app.vue`, `components/`, `composables/`, `pages/`, `layouts/`, `plugins/`, `stores/`); `nuxt.config.ts`, `i18n/`, `test/` and generated `gen/` stay at the package root. `~`/`@` point at `app/`; `~~`/`@@` at the root.
- Data comes from typed Connect clients provided by `app/plugins/connect.client.ts` and called from Pinia setup stores, not from `useFetch` against `/api`.
- Auto-imports are narrowed: `components: { dirs: [] }` and `imports: { scan: false }`. Vue, Nuxt and module APIs stay global.
- The console extends the `ui/base` Nuxt layer (`extends: ['@oagent/base']`), which brings `@nuxt/eslint`, `@nuxt/icon`, `@pinia/nuxt`, `@nuxtjs/i18n` (`no_prefix` strategy, `en-US` and `id-ID`), the shared components, composables, locales, CSS and the `#base`, `#gen`, `#test` aliases; the console adds `@vueuse/nuxt`.
- Routes use slugs in the path (`/o/<slug>/...`); every id shown or sent is a public id.
- `runtimeConfig.public` (`apiUrl`, `appVersion`) stays empty at build time; the Go handler fills it in per request.
- See docs/howto/console-page.md for adding a page end to end.
