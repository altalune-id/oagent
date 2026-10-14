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
- **Layering.** Domains import only `internal/kernel/*`, root packages, other domains' ports and `internal/web/layout`. depguard enforces part of this; the rest is review.
- **Features pick a driver; connections are root libraries.** A feature lives in `internal/kernel/<feature>`, names its driver in its own block (`db.driver`, `rateLimit.driver`, `queue.driver`) and takes a driver-agnostic interface (`ratelimit.Limiter`, `queue.Queue`) or, for the database, `db.Pool` and `db.Transactor`. A feature with several backends keeps each in a sub-package (`queue/nats`, `ratelimit/memory`, `ratelimit/redis`); one that must commit with domain data (`outbox`, `mailqueue`, `session`) keeps its Postgres store in-package. The connection is its own block (`postgres.*`, `redis.*`, `nats.*`) and a root package (`pg`, `redis`, `nats`) that opens, health-checks and closes it and returns the real client; boot opens it only when `<Backend>Features()` lists a user. Kernel packages and connections never import `config`: one that needs settings has its own `Config` in `config.go`, mapped by boot (depguard `config-boundary`). Only the connection, its drivers, boot and test helpers import pgx, go-redis or NATS (`pgx-boundary`, `redis-boundary`, `nats-boundary`); domains use `db.IsUniqueViolation`, `db.IsForeignKeyViolation` and `db.ViolatedConstraint`, and never import `pg`, `nats` or `redis`. On a shared NATS server each app has its own account; names are not prefixed.
- **Error codes are append-only** and translated in every console locale.
- **Fresh-database migrations.** No backfills, guards or SQL id generators.
- **READMEs move with the code.** `schema/README.md` for schema changes, `internal/boot/README.md` for wiring changes, in the same commit.
- **Comments.** Default none; follow the `comment-discipline` skill. `make comment-check` must stay at 0.
- **Style.** Go: gofmt + gci import groups. TS/Vue: ESLint with semicolons; interface fields end with `;`, inline object types use `,`; never a one-line `if`.

## 4. Build

- Copy the shape of the reference code each recipe links. Prefer an existing helper over a new one.
- TDD: write the failing test first (reach, permission, validation, public-id tests included), then the code.
- Go: follow the `go` and `go-concurrency` skills, and `go-proverb` for judgment calls.
- UI: follow `vue`, `pinia`, `nuxt`, `vitest` and `eslint-config`.

## 5. Verify

```sh
.agents/skills/oagent-go-convention/scripts/verify.sh                # regenerate, then every unit gate
.agents/skills/oagent-go-convention/scripts/verify.sh --integration  # plus make cover (Postgres via testcontainers)
.agents/skills/oagent-go-convention/scripts/verify.sh --ui           # plus typecheck, UI tests, build, Storybook
```

Then open what you changed: a console page in the browser, a CLI command in a terminal, an MCP tool from a client. A green build does not prove a surface works.

## Never

- Commit, stage or push. The user commits.
- Add AI attribution anywhere.
- Edit generated code (`gen/`, `ui/console/gen/`, `*_templ.go`) by hand.
- Weaken a linter, a coverage floor or a test to get green.
