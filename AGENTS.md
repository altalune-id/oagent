# AGENTS.md

Rules for anyone changing oagent, people and coding agents alike. `CLAUDE.md` is a link to this file.

oagent is one Go binary (module `altalune.id/oagent`) that serves an API (Connect RPC), an MCP server, a CLI and a web console. The UI lives in the pnpm workspace `ui/*` and is embedded into the binary.

Setup: see [README.md](README.md). Architecture: [docs/architecture](docs/architecture/README.md).

## Before you change anything

1. Find the recipe for your change in [docs/howto](docs/howto/README.md) (RPC, MCP tool, CLI command, migration, config key, error code, console page and so on).
2. Follow its **Plan / Build / Review** checklists. A recipe names every file to touch.
3. Copy the shape of the reference code the recipe links to. Don't invent a new pattern; if none fits, ask.

## Gates

Run these before you hand work over. CI runs the same ones.

| Command                                | When                                                                                              |
| -------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `make check`                           | always (vet, every linter, unit tests)                                                            |
| `make comment-check`                   | always (comment discipline, 0 violations)                                                         |
| `make cover`                           | Go changes (unit + integration, at least 85% per package)                                         |
| `make ui-test` and `pnpm -r typecheck` | `ui/` changes (console: 90% lines, 85% branches)                                                  |
| `make generate`                        | after editing `.proto`, `.templ`, `scope/scopes.json` or `apperror/codes.json`; commit the output |
| `make config-examples`                 | after changing a config struct tag                                                                |
| `make ui`                              | to build the embedded UI before running or testing it                                             |

Fix the cause of a failing gate. Never skip a hook (`--no-verify`) or weaken a linter rule to get green.

## Rules

**Identifiers**

- Every identifier on a surface (API, MCP, CLI, console API calls, webhook payloads, user-facing errors) is the `public_id`.
- UUIDs stay internal. Slugs appear only in URL paths. See [public-ids](docs/howto/public-ids.md).

**Layout and imports**

- Import direction: `cmd` → `internal/cli` → `internal/boot` → `internal/controlplane` → domains → `internal/kernel/*`.
- Domains never import `internal/cli`, `internal/boot`, `internal/controlplane` or `internal/web` (only `internal/web/layout`).
- Root packages (`apperror`, `config`, `mailer`, `worker` and others) never import `internal/`. The connections are root packages too, generic libraries with no oagent knowledge: `pg`, `nats`, `redis` open, health-check and close one backend each, return the real client (`*sql.DB`, `*nats.Conn`, the go-redis client) and never wrap its API.
- Features that services use live in `internal/kernel/<feature>`. A feature that may have several backends picks one with a `driver` key and keeps each in a sub-package (`queue/nats`, `ratelimit/memory`, `ratelimit/redis`). A feature that must commit with domain data (`outbox`, `mailqueue`, `session`) keeps its Postgres store in-package.
- Kernel features and connections never import `config`: one that needs settings takes its own `Config` (in its `config.go`), and boot maps the app config into it. Boot opens a connection only when a feature selects it.
- Only `pg`, `internal/kernel/db`, `migrator` and `pgtest` import the Postgres driver; domains use `db.IsUniqueViolation`, `db.IsForeignKeyViolation` and `db.ViolatedConstraint`. Domains never import `pg`, `nats` or `redis` (depguard).
- A domain package holds only the files it needs: `<domain>.go` (aggregate, no I/O), `errors.go`, `ports.go`, `store.go` + `postgres.go`/`pgreader.go`/`pgwriter.go`, `service.go`, then one file per surface: `controlplane.go`, `cli.go`, `web.go`. See [module](docs/howto/module.md).
- Dev tools live in `internal/tool/`; their commands in `cmd/`.

**Surfaces**

- Signed-in surfaces (Connect, MCP, signed-in pages) call the `controlplane` handlers, never a service directly.
- Every RPC has an entry in the permission table (`internal/permission`) and protovalidate rules in its proto.
- Only pre-sign-in pages (login, install, invite landing, legal) are templ pages. Everything signed-in goes in the console.

**Data**

- Migrations assume a fresh database: no backfills, guards or SQL id generators.
- A migration that changes a table, column, constraint or index updates `schema/README.md` in the same commit.
- A change to domains, surfaces, workers or boot order updates `internal/boot/README.md` in the same commit. It describes the current wiring only, with no history.

**Errors**

- A new error code means all of these, in the same commit:
  - an `apperror/codes.json` entry, then `make generate`
  - a line in `apperror/issued_codes.txt`
  - translations in every console locale
- Codes are append-only: never delete, rename or reuse one. See [error-code](docs/howto/error-code.md).

**Config**

- `.env.example` and `config.example.yml` hold only what is required to start. Optional keys go in the reference files (`make config-examples`).
- A feature and its connection are separate top-level keys: the feature picks its backend with `driver` (`db.driver: postgres`, `queue.driver: nats`, `rateLimit.driver: memory|redis`); the connection holds everything backend-specific (`postgres.*`, `nats.*`, `redis.*`) and `<Backend>Features()` lists who uses it. See [config-key](docs/howto/config-key.md).

**Style**

- Go: gofmt + gci import groups (stdlib, third-party, `altalune.id/oagent`), golangci-lint config in `.golangci.yml`.
- JS/TS/Vue/JSON/Markdown/CSS:
  - ESLint (`@antfu/eslint-config`) is the formatter too; run `pnpm lint:fix`.
  - Statements end with `;`. Interface fields end with `;`; inline object types use `,`.
  - No one-line `if`: always braces, on their own lines.
- YAML: yamllint. Dockerfiles: hadolint. Workflows: actionlint. Shell: shellcheck. They run through `scripts/lint/tool.sh`: the installed binary, else Docker, else skipped with a note.
- Comments:
  - The default is no comment.
  - Exported symbols get a one-sentence doc.
  - Keep only markers (`TODO`, `NOTE`, `SECURITY`…) or links. No rationale, history or section headers.
- Generated code (`gen/`, `*_templ.go`, `ui/console/gen/`) is never edited by hand.

**Versions**

- Go 1.26.9, Node 24.15 or newer, pnpm 10 (never 11).
- Use the latest stable dependency versions that are at least a few days old.
- Docker images are pinned by version, not digest.

## Git

- Conventional Commits, one line (`feat(scope): …`), template in `.gitmessage.txt`.
- Every commit is signed (GPG or SSH). The pre-push hook refuses unsigned commits.
- No AI attribution anywhere: no `Co-Authored-By` trailers, no "Generated with" lines, in commits, PRs, code or docs.
- Playwright output stays in `.playwright-mcp/` (git-ignored); delete it after use.

## Agent skills

Skills in `.agents/skills/` (each linked from `.claude/skills/`) are committed and carry the same conventions. The linters skip them:

- `oagent-go-convention`: build a change the oagent way.
- `oagent-go-review`: review a change; it runs the gates.
- `go`, `go-concurrency`, `go-proverb`, `comment-discipline`: Go and comment style.
- `vue`, `pinia`, `vite`, `vitest`, `nuxt`, `eslint-config`: the console and the UI toolchain.
- `daisyui`: daisyUI 5 classes, colors and themes for the console and the sign-in pages.
- `cobra-viper`, `go-release`, `go-spec-reviewer`: CLI and config, releases, spec review.

A new skill goes in `.agents/skills/<name>/` with a `.claude/skills/<name>` link, in the same commit.
