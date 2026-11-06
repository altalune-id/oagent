# Architecture

oagent is a self-hosted platform for AI agents. One Go binary (`oagent`, module `altalune.id/oagent`) serves every surface from one set of domain services and one Postgres database. A second container, the agent runtime (`apps/runtime`), runs the agent turns.

For step-by-step changes, see the recipes in [docs/howto](../howto/README.md).

## Surfaces

| Surface           | Where                                                    | Code                                                                  |
| ----------------- | -------------------------------------------------------- | --------------------------------------------------------------------- |
| API               | `/api` (Connect RPC, OpenAPI docs)                       | `internal/controlplane`, protos in `api/`                             |
| Data plane        | `/api/v1/agents/{agent}/...` (AG-UI SSE, bearer API key) | `internal/dataplane`; see [data-plane.md](../data-plane.md)           |
| MCP server        | `/mcp` (HTTP) and `oagent mcp` (stdio)                   | generated tools over the Connect handlers, `internal/mcp`, root `mcp` |
| CLI               | `oagent ...`                                             | `internal/cli` (shell) + each domain's `cli.go`                       |
| Web console       | `/console`                                               | Nuxt SPA in `ui/console`, embedded by `internal/web/console`          |
| Pre-sign-in pages | `/login`, `/install`, `/invite`, ...                     | each domain's `web.go` + templ in `<domain>/views`                    |
| Probes            | `/healthz`, `/readyz`                                    | `internal/health`                                                     |

Every signed-in surface (API, MCP, console) reaches a domain through the `controlplane` handlers, which hold the permission and reach checks. The data plane is the integrator's surface: it takes only an API key, checks reach and its `run:*` scopes in its own handler, and drives the run domain's store, runner and cancel directly, because a streamed turn is not an RPC. Only pre-sign-in pages, operator commands that boot the `App`, and workers call services directly.

## Containers

| Container                                                       | Owns                                                                                                                          | Never                                                       |
| --------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| `oagent` (Go)                                                   | every surface, tenancy, threads, messages, snapshots, runs, end users, providers, MCP servers, sealed secrets, the turn queue | runs the model                                              |
| runtime (TypeScript, `apps/runtime`, package `@oagent/runtime`) | one agent turn: builds a Strands agent from the request, calls the model and the MCP tools, streams the turn back             | stores anything, holds database credentials, calls `oagent` |

The flow is one-way. `oagent` posts a turn to the runtime's `POST /turn` with a shared bearer, and reads back an NDJSON stream: AG-UI events plus private checkpoint, heartbeat and terminal events. Everything a turn needs travels in the request, and everything it produces travels in the stream. The request and stream contract lives in `internal/kernel/runtime/contract.go`, with goldens in `internal/kernel/runtime/testdata/turn/` that the Go tests and the runtime's tests both read. Every runtime egress (model, MCP, each redirect hop) goes through one guarded dispatcher that refuses private addresses, with test vectors shared with Go in `httpclient/testdata/ssrf_vectors.json`. The runtime listens on the private network only. See [apps/runtime/README.md](../../apps/runtime/README.md).

## Layers

```mermaid
flowchart TB
  CMD["cmd/oagent"] --> CLI["internal/cli<br/>root command, flags, config"]
  CLI --> BOOT["internal/boot<br/>composition root"]
  BOOT --> CP["internal/controlplane<br/>Connect handlers, permission, validation"]
  BOOT --> WEB["internal/web<br/>mux, static, gates (no domain)"]
  CP --> DOM["domains<br/>org, project, agent, modelprovider, mcpserver, enduser, run, apikey, webhook, user, auth, install, legal, health"]
  WEB -. Registrar .-> DOM
  DOM --> LAYOUT["internal/web/layout<br/>templ shell"]
  DOM --> KERNEL["internal/kernel/*<br/>authn, authz, session, tenant, events, capabilities<br/>features: db, outbox, mailqueue, queue, ratelimit, runtime"]
  DOM --> ROOT["root packages<br/>apperror, mailer, worker, scheduler, ..."]
  KERNEL --> ROOT
  BOOT --> CONN["connections (root)<br/>pg, nats, redis"]
  BOOT --> CFG["config"]
```

Import direction: `cmd` → `internal/cli` → `internal/boot` → `internal/controlplane` → domains → `internal/kernel/*`. Domains never import `internal/cli`, `internal/boot`, `internal/controlplane` or `internal/web` (only `internal/web/layout`). Kernel packages import no domain (depguard `kernel-boundary` in `.golangci.yml`). Kernel packages and connections never import `config` (`config-boundary`): each takes its own `Config` from its `config.go`, and boot maps the app config into it.

## Boot

`internal/boot` turns `config.Config` into a running `App`: it opens the database, builds every service once, wires the surfaces and starts the workers. The order, request flow, service wiring, rate limits, Redis, NATS, the queue and workers are in [internal/boot/README.md](../../internal/boot/README.md).

## Kernel

`internal/kernel` holds what services use. It imports no domain. Two kinds of package live there.

The app core:

- `authn`, `authz`: who the caller is, and what they may do.
- `session`: the `Principal` and the browser session store (memory and Postgres stores in-package).
- `tenant`: the org, project and user on the request.
- `events`: the webhook event catalog.
- `capabilities`: the feature switches derived from the deployment mode.

The features, each picking its backend with a `driver` key:

- `db` (`db.driver: postgres`): the writer/reader `Pool`, the unit of work (`RunInTx`, `InTx`, `LockInTx`) and the Postgres error checks (`IsUniqueViolation`, `IsForeignKeyViolation`, `ViolatedConstraint`), so domains never import the driver.
- `outbox`, `mailqueue`: durable delivery (webhooks, mail). They commit in the same transaction as the domain change, so they keep their Postgres store in-package and have no driver switch.
- `queue` (`queue.driver: nats`): jobs and broadcasts, driver-agnostic; its NATS JetStream driver is `queue/nats`.
- `ratelimit` (`rateLimit.driver: memory|redis`): GCRA limits, with `ratelimit/memory` and `ratelimit/redis` drivers and the shared contract in `ratelimit/ratelimittest`.
- `runtime` (`runtime.driver: http`): `Runtime.Turn` posts one turn and returns its event `Stream`. The turn contract and its goldens are in-package, and the HTTP driver is `runtime/http`.

## Connections

The root packages `pg`, `nats` and `redis` each open, health-check and close one backend, then return the real client (`*sql.DB`, `*nats.Conn`, the go-redis client) without wrapping its API. Their config is a top-level key (`postgres.*`, `nats.*`, `redis.*`) mapped by boot into the package's own `Config`. Boot opens a connection only when a feature selects it (`PostgresFeatures`, `NATSFeatures`, `RedisFeatures`), hands it to the feature, and closes it on shutdown; `/readyz` reads each one's health monitor.

Only the connection, its drivers, boot and the test helpers import a backend library (depguard `pgx-boundary`, `nats-boundary`, `redis-boundary`). Domains never import `pg`, `nats` or `redis`.

## Domains

Each domain is a package under `internal/`: `org` (with `org/member` and `org/invite`), `project`, `agent`, `modelprovider`, `mcpserver`, `enduser`, `run`, `apikey`, `webhook`, `user`, `auth`, `install`, `legal`, `health`. `enduser` holds the people agents talk to and their identities per project. `run` holds threads, messages, snapshots and runs: the ordered inbox, the thread lease, the turn runner and the run's usage and cost. Each uses only the files it needs: `<domain>.go`, `errors.go`, `ports.go`, `store.go` + `pg*.go`, `service.go`, then one file per surface (`controlplane.go`, `cli.go`, `web.go`), with templ views in `<domain>/views`. See [module.md](../howto/module.md).

Other internal packages: `permission` (the permission each RPC needs), `apiclient` (the CLI's Connect client), `prompt` (stdin prompts), `render` (CLI output), `mcp` (MCP auth and payloads).

## Root packages

Packages at the repo root never import `internal/`: `apperror` (error codes, `codes.json`), `config`, the connections `pg`, `nats` and `redis`, `migrator`, `mailer` (+ console, smtp, resend), `worker`, `scheduler`, `httpclient`, `httpserver`, `mcp`, `password`, `sealer`, `nanoid`, `slug`, `weburl`, `scope`, `reqid`, `telemetry`, `version`.

## UI builds

The pnpm workspace holds `ui/*`, whose builds the binary embeds, and `apps/*`, which ship as their own images (`apps/runtime`). The builds are not committed.

| Package      | What                           | Build             | Embedded at                   |
| ------------ | ------------------------------ | ----------------- | ----------------------------- |
| `ui/console` | Nuxt 4 console SPA             | `make ui-console` | `internal/web/console/public` |
| `ui/base`    | shared Nuxt layer              | `make ui-base`    | (extended by `ui/console`)    |
| `ui/mcp`     | MCP Apps view                  | `make ui-mcp`     | `internal/mcp/ui/assets`      |
| `ui/web`     | Tailwind CSS + password script | `make web`        | `internal/web/static`         |

Every surface shares one look: `ui/theme.css` holds the daisyUI themes, and `ui/base` (a Nuxt layer) holds the shared components. `ui/mcp` is plain Vite + Vue in a sandboxed iframe, so it imports `ui/base/app/components/ui/*` source directly (the `@oagent/base` workspace dev dependency and `baseAliases`) and registers the two Nuxt globals those components use with `installStandalone`. A test keeps `components/ui` free of Nuxt composables, and the MCP build inlines only the icons its sources name.

`make ui` builds all three. `ui/scripts/compress.mts` (package `@oagent/scripts`) then writes a `.br` and `.gz` next to each console and sign-in text file over 1 KiB. See [console-page.md](../howto/console-page.md) and [web-page.md](../howto/web-page.md).

## Data

Postgres only (`db.driver: postgres`, connection `postgres.*`). Migrations live in `schema/migrations` and run through `migrator` (at boot with `db.autoMigrate`, on `postgres.migrator.dsn` when set). Tables, keys and constraints are in [schema/README.md](../../schema/README.md). See [migration.md](../howto/migration.md).

## Code generation and dev tools

- `make generate`: protos (Go, Connect, OpenAPI, MCP tools, TypeScript in `ui/base/gen`), scope names, error codes and templ.
- `cmd/*`: the generators and linters (`gen-apperror`, `gen-scope`, `gen-config-example`, `protoc-gen-mcp`, `comment-lint`). `protoc-gen-mcp` and `comment-lint` keep their code in `internal/tool` (`protocmcp`, `commentlint`).
- `make check`: vet, lint and tests.
