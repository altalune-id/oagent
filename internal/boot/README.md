# boot

`boot` is the composition root. It turns `config.Config` into a running app:
it opens the database, builds every domain service, wires the surfaces onto
them and starts the workers. No other package wires domains together.

Keep this file in step with the code. When a domain, surface or worker is
added or removed, update the diagrams and the tables below in the same commit.

## Boot order

`Boot` (`boot.go`) runs these steps in order. Any error stops the boot.

```mermaid
flowchart TD
    A[setupTelemetry] --> B[openPool]
    B --> C{db.autoMigrate?}
    C -- yes --> D[migrate]
    C -- no --> E
    D --> E[kernel: log, pool, tracer, meter, reporter, queue]
    E --> F[buildServices: every domain service, the mail queue]
    F --> M[buildMailer: mail.driver picks mailer/console, mailer/smtp or mailer/resend]
    M --> G[buildAuthSecrets: state secret, setup token]
    G --> H[firstBoot: basic system org / env genesis / log install URL]
    H --> I[installRequired: is the install gate shut?]
    I --> J[build surfaces: API, console, MCP, web routes]
    J --> K[supervisor: register workers]
    K --> L[App]
```

`App.Run` starts the supervisor (`serve`). `App.RunMCP` serves MCP over stdio
instead (`oagent mcp`). `App.SetPassword` (`oagent user set-password`) and
`App.AddMember` (`oagent member add`) are operator actions, booted with the
install URL, scheduler and consumer off.
`BootRemote` (`remote.go`) is the CLI side: config, a logger and a Connect
client (`internal/apiclient`) to a running server, with no database.

## Request flow

Each surface goes through its own chain, then reaches the same domain
service. Every service writes through its own store.

```mermaid
flowchart TD
    CLI["oagent CLI"] -- Connect client --> API
    CLI -- "auth login / logout" --> WEB
    API["/api"] --> G1[InstallGate] --> C1[CrossOrigin] --> B1["BasicChallenge (basic mode)"] --> I1["RequestID, OTel, Wrap"] --> I2["authn + permission"] --> I3[tenantFromPrincipal] --> DC
    WEB["web routes"] --> G2[InstallGate] --> C2[CrossOrigin] --> S2[Session] --> W2[WelcomeGate] --> DW
    CON["/console"] --> G3[InstallGate] --> S3[Session] --> W3[WelcomeGate] --> CG3["ConsoleGate: /login redirect or Basic challenge"] --> SPA[console SPA]
    SPA -. calls .-> API
    MCPH["/mcp HTTP"] --> G4[InstallGate] --> A4["mcp.Authenticate: key or Basic, no cookie"] --> T4["tool scope check"] --> DC
    MCPS["oagent mcp stdio"] --> SYS["session.System() Principal"] --> T4
    LB["/healthz, /readyz"] --> HW["health web.go (no gate, session or CSRF)"] --> HS["health service.go"] --> DBH["db-health monitor"]

    DC["domain controlplane.go"] --> SVC["domain service.go"]
    DW["domain web.go"] --> SVC
    SVC --> ST["domain store.go, pg*.go"] --> PG[(Postgres)]
```

- **Reach:** after the permission check, each `controlplane.go` checks that the caller reaches the org, project or resource (`kernel/authz.Reach*`). Out of reach answers 404.
- **MCP:** tools are generated from the same Connect services, so MCP and `/api` share one handler per RPC.
- **Credentials required:** `requireCredentials` (`http.go`) makes `/api` and `/mcp` answer 401 and the console redirect to `/login?return_to=…` (local) or send a Basic challenge (basic mode) when no credential is sent. `/login`, `/logout`, `/install`, `/welcome`, `/terms`, `/privacy`, `/static`, `/healthz` and `/readyz` stay open. A request with no Principal reaches nothing (`kernel/authz`).
- **Permissions:** `internal/permission` is the one table of the permissions each RPC needs (all of them; a missing or empty entry is denied). The `/api` interceptor reads it, and every generated MCP tool takes its RPC's entry (`permission.For` in `mcp.go`).
- **MCP scopes:** `tools/list` and `tools/call` both check every one of the tool's permissions against those the caller holds (`mcpinternal.ScopesFromContext`). No Principal holds none.
- **stdio:** `oagent mcp` runs as `session.System()`, an operator, never as "no Principal".
- **Who calls services directly:** any signed-in surface (Connect, MCP, a signed-in web page) calls the `controlplane` handlers, never a service, because the handlers hold the permission and reach checks; services check domain rules only. Only pre-sign-in pages (login, install, invite landing), operator commands that boot the `App`, and workers call services directly. `reach_integration_test.go` calls every RPC from another org, and with a key limited to another project of the same org (only `GetOrg`, `ListOrgs` and `ListProjects` answer; the list holds only what the key reaches).

## Domains and their surfaces

| Domain | Service deps (ports) | Connect + CLI | MCP tools | Web routes |
|---|---|---|---|---|
| `org` | — | ✓ | ✓ | — |
| `org/member` | effects: sessions, apikey (personal keys), user (contact), org (name), mail queue | ✓ (role changes and removal: people and the operator only; leave: the signed-in person); CLI `member add` (operator, boots the `App`) | `member_list` | — |
| `project` | — | ✓ | ✓ | — |
| `agent` | webhook (publisher), project (org of project) | ✓ | ✓ | — |
| `apikey` | project (ids), member (owner roles) | ✓ | — | — |
| `webhook` | project + org (refs), project (org of project), outbox | ✓ | ✓ | — |
| `user` | — | CLI `user set-password` (operator, boots the `App`) | — | — |
| `auth` | user, member, org, project, sessions, passwords | whoami; CLI `auth login/logout/whoami` (profiles through `CLIProfiles`) | `auth_whoami` | `/login`, `/logout` |
| `install` | user (create, accept terms), org, member, project; auth (sign-in) | — | — | `/install` (terms and privacy consent, `layout.Consent`) + `InstallGate` |
| `legal` | user (accept terms), auth (rotate session) | — | — | `/terms`, `/privacy`, `/welcome` + `WelcomeGate` (asks again when the terms changed after the person accepted them) |
| `health` | readiness checks (db-health monitor) | — | — | `/healthz`, `/readyz` (probes slot, root-anchored) |

Every domain follows the same file layout: `<domain>.go`, `errors.go`,
`ports.go`, `store.go` + `pg*.go`, `service.go`, then one file per surface
(`controlplane.go`, `cli.go`, `web.go`), with templ in `<domain>/views`.

`internal/cli` is only the CLI shell: the root command, flags, config, and the
URL, token, profile and session-file resolution. Each domain's command tree is
its `cli.go` (`NewCmd`). Domains never import `internal/cli`, `internal/boot`,
`internal/controlplane` or `internal/web` (only `internal/web/layout`); the CLI's
Connect client lives in `internal/apiclient`, which imports no domain.

## Service wiring

`buildServices` (`services.go`) builds each service once and passes it to
whoever needs it. An arrow means "depends on". Small adapters in `services.go`
(`projectIDs`, `projectOrgs`, `ownerRoles`, `refResolver`, `memberPeople`, `memberDirectory`,
`memberOrgs`) fit one service to another domain's port.

```mermaid
flowchart TD
    subgraph Flows["sign-in flows"]
        install
        legal
        auth
    end
    subgraph Ops
        health
    end
    subgraph Resources
        agent
        webhook
        apikey
        keyAuthn["apikey.Authenticator"]
    end
    subgraph Core
        org
        project
        member
        user
        sessions[(session store)]
        mailqueue[(mail queue)]
    end

    install --> auth
    legal --> auth
    install --> user & org & member & project
    legal --> user
    auth --> user & member & org & project & sessions

    agent --> webhook
    agent --> project
    webhook --> project & org
    apikey --> project & member
    keyAuthn --> member
    member -. effects .-> sessions & apikey & user & org & mailqueue
    health --> dbh[(db-health monitor)]

    org --> project
```

`org → project` is the after-create hook that adds the default project.
`member → …` (dotted) are the effects set by `memberEffects` after every service exists
(`apikey` already depends on `member`). A removal ends the person's sessions, revokes their
personal keys in that org and queues a notice; a role change queues a notice and, when it lowers
the role, ends their sessions; leaving revokes their personal keys. All of it runs in the
membership change's transaction. Notice links start at `publicBase(cfg.HTTP)` (also used for
the install URL). `member` gets `WithOperatorSupport(caps.OperatorOrgSupport)` (on in cloud,
off in selfhosted): staff (an operator who is a person) follow their role in orgs they belong
to, and manage other orgs with the admin permission set only when it is on (selfhosted refuses
them there). No one acts on or grants more permissions than they hold, so only owners touch
owners. The stdio System principal, basic auth and the CLI `App` keep full operator power.
`WithDirectory(memberDirectory{…})` lets `member add` find the user by email and the org by
slug.
`install → auth` and `legal → auth` exist only on the web side, wired in
`http.go`. `buildLegal` builds `Services.Legal`. The `WelcomeGate` asks again when a person
accepted before the built-in documents' newest `updated` date; with `compliance.termsURL` or
`compliance.privacyURL` set it uses `compliance.termsUpdatedAt`, which may not be in the future.

## Background workers

The supervisor runs these next to the HTTP server.

| Worker | What it does | When it runs |
|---|---|---|
| `http` | the HTTP server (all surfaces above) | always |
| `db-health` | pings the pool; `health` reads it for `/readyz` | always |
| `outbox` | claims due outbox entries, delivers signed webhooks | always |
| `mailqueue.send` | claims due queued mail, sends it through the mailer, erases the sealed payload once sent or failed | always |
| `apikey-usage` | batches `last_used_at` writes every 30 s | always |
| `scheduler` | cron jobs (expired-session sweep) | `scheduler.enabled`, unless `--no-scheduler` or consumer-only |
| queue consumer / listener | queue handlers (none yet) | when `queue.enabled` |

## Files

| File | Builds |
|---|---|
| `boot.go` | `Boot`: the order above |
| `services.go` | `Services` and every domain service, plus the port adapters |
| `auth.go` | secrets, session store, first boot, install state |
| `http.go` | API and MCP handlers and their authn chains, web routes, session middleware, CSRF, the HTTP handler |
| `mcp.go` | MCP server and its tools |
| `console.go` | console SPA handler |
| `outbox.go`, `schedulers.go`, `consumers.go`, `queue.go` | workers |
| `mail.go` | the mailer driver (`mailer/console`, `mailer/smtp` or `mailer/resend`, picked by `mail.driver`), the sealed mail queue (`Services.MailQueue`) and its send worker |
| `db.go`, `sealer.go`, `telemetry.go` | pool, migrations, health, sealer, telemetry |
| `options.go`, `app.go`, `remote.go` | boot options, the running `App` and its operator actions, the CLI graph |

## Add a domain

1. Create `internal/<domain>` with the files above. Put its consumed interfaces in `ports.go`.
2. In `services.go`, add the store and service to `Services` and build them in `buildServices`.
3. For each surface it has:
   - **Connect:** `controlplane.Services` + `server.go`, and every RPC in `internal/permission`.
   - **CLI:** `<domain>/cli.go` (`NewCmd`), added in `internal/cli/root.go`.
   - **MCP:** `mcp.go`.
   - **Web:** `buildRoutes` in `http.go`.
   - **Readiness:** pass its check to `health.NewService` in `boot.go`.
4. Add a worker or scheduler job in `boot.go` / `schedulers.go` if it needs one.
5. Update this README: both diagrams, the domains table and the workers table.
