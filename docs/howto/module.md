# Add a domain module

Use this when oagent needs a new bounded context: a new aggregate with its own table, rules and surfaces, in a new package under `internal/`. To add behaviour to a domain that already exists, use [service-method.md](service-method.md) and [store-method.md](store-method.md). A second aggregate that belongs to an existing context is a subdomain in a child directory, like `internal/org/member` and `internal/org/invite`.

## Plan

- [ ] **New domain or subdomain?** It is a subdomain when it shares role or error types with a parent (`org/member`, `org/invite`). Otherwise a new top-level package.
- [ ] **Scope.** Which org (and project) owns a row? Every tenant row carries `org_id`, and `project_id` when it lives in a project.
- [ ] **Identifiers.** The aggregate gets a `public_id` (21-character nanoid). Surfaces use only that. A slug, if any, is for URL paths only. See [public-ids.md](public-ids.md).
- [ ] **Surfaces.** Pick only what it needs: Connect ([rpc.md](rpc.md)), MCP ([mcp-tool.md](mcp-tool.md)), CLI ([cli-command.md](cli-command.md)), console ([console-page.md](console-page.md)), pre-sign-in page ([web-page.md](web-page.md)).
- [ ] **Ports.** What does it need from other domains? Each need is a small interface in its own `ports.go`, filled in boot by the other service or an adapter. It never imports another domain to call it.
- [ ] **Units of work.** Does a use case write more than one row, or a row plus an event? Then the service takes a `db.Transactor`.
- [ ] **Events.** Does it emit webhook events? See [webhook-event.md](webhook-event.md).
- [ ] **Errors.** List the failures a user can see; each one is a typed error and an error code ([error-code.md](error-code.md)).

## Build

1. **Migration.** `schema/migrations/NNN_<name>.sql` at the next number, bump `schema/migrations/VERSION`, add the jet binding in `schema/entity/<table>.go`, and update `schema/README.md` (diagram and constraints table). Follow [migration.md](migration.md). The table shape copies `003_agents.sql`:

   ```sql
   id          UUID PRIMARY KEY,
   public_id   TEXT NOT NULL UNIQUE,
   org_id      UUID NOT NULL REFERENCES {{.Schema}}.{{.TablePrefix}}orgs (id) ON DELETE CASCADE,
   created_at  TIMESTAMPTZ NOT NULL,
   updated_at  TIMESTAMPTZ NOT NULL
   ```

2. **Domain files**, in `internal/<domain>/`. Create only those it needs:

   | File                                        | Holds                                                                                   |
   | ------------------------------------------- | --------------------------------------------------------------------------------------- |
   | `<domain>.go`                               | the aggregate, its constructor and verbs, the row struct (`toX`) and `xToProto`. No I/O |
   | `errors.go`                                 | typed errors, `Is<Name>Error`, `IsExpectedError`, `ToAppError`                          |
   | `ports.go`                                  | interfaces it consumes from other domains (`Publisher`, `Refs`…)                        |
   | `store.go`                                  | `Store` = `Reader` + `Writer` interfaces                                                |
   | `postgres.go`, `pgreader.go`, `pgwriter.go` | `postgresStore`, `NewStore(pool, schema, tablePrefix)`, reads, writes                   |
   | `service.go`                                | `Service`, `NewService(...)`, `With*` options, use cases                                |
   | `controlplane.go`                           | the Connect handler, `NewControlPlane(svc, refs)`; resolves public ids and checks reach |
   | `cli.go`                                    | `ClientFn` and `NewCmd(clientFn)`                                                       |
   | `web.go`, `views/`                          | only for pre-sign-in pages                                                              |

   The constructor mints the ids, copied from `agent.New`:

   ```go
   publicID, err := nanoid.New(21)
   if err != nil {
       return nil, err
   }
   a := &Agent{ID: uuid.New(), PublicID: publicID, OrgID: orgID, CreatedAt: now, UpdatedAt: now}
   ```

3. **Store and service.** Follow [store-method.md](store-method.md) per verb and [service-method.md](service-method.md) per use case.
4. **Test double.** `internal/testutil/fakes/<domain>.go` with `var _ <domain>.Store = (*<Name>Store)(nil)`, like `fakes.AgentStore`.
5. **Lint scope.** Add the package name to both `domain-purity` globs in `.golangci.yml`. That rule keeps `<domain>.go`, `store.go` and `errors.go` free of I/O imports.
6. **Boot.** In `internal/boot/services.go`: a field on `Services`, the store and service built in `buildServices`, adapters for its ports. Pass `uow` when it writes more than one row. If its public ids must be resolved by other domains, extend `publicIDs` in `internal/boot/publicids.go`.
7. **Surfaces.** For Connect: `Services`, `Server` and `New` in `internal/controlplane/server.go`, the `Refs` interface there, and every RPC in `internal/permission/permission.go`. For MCP: `internal/boot/mcp.go`. For CLI: `NewCmd` added in `internal/cli/root.go`. For web: `buildRoutes` in `internal/boot/http.go`.
8. **Boot README.** Update `internal/boot/README.md` in the same change: the domains table, the service wiring diagram and the files table. Describe the current wiring only.
9. **Gates.** `make generate` (after protos), `make check`, `make comment-check`, `make test-integration`, `make cover` (each package at least 85%).

## Review

- [ ] Files sit where the layout says, not only where the plan said. No command file in `internal/cli`; no surface code in `service.go`.
- [ ] Imports point down only: the domain does not import `internal/cli`, `internal/boot`, `internal/controlplane` or `internal/web` (only `internal/web/layout`). `internal/kernel/*` does not import it.
- [ ] Another domain is reached only through a port in `ports.go`, wired in boot.
- [ ] Signed-in surfaces (Connect, MCP, console) go through `controlplane.go`; only pre-sign-in pages, operator commands and workers call the service directly.
- [ ] Every id on a surface is a public id; the reach and no-UUID tests cover the new RPCs ([public-ids.md](public-ids.md)).
- [ ] The migration, `schema/entity`, `schema/README.md` and `internal/boot/README.md` agree with the code.
- [ ] The package is added to `domain-purity` in `.golangci.yml`.
- [ ] Coverage of the new package is at least 85% under `make cover`.

## Reference code

- Aggregate: [internal/agent/agent.go](../../internal/agent/agent.go) (`Agent`, `New`, `agentRow`, `agentToProto`)
- Errors: [internal/agent/errors.go](../../internal/agent/errors.go) (`NotFoundError`, `IsExpectedError`)
- Ports: [internal/agent/ports.go](../../internal/agent/ports.go) (`Publisher`, `Refs`)
- Store: [internal/agent/store.go](../../internal/agent/store.go), [internal/agent/postgres.go](../../internal/agent/postgres.go) (`NewStore`)
- Service: [internal/agent/service.go](../../internal/agent/service.go) (`NewService`, `WithPublisher`, `WithProjects`)
- Handler: [internal/agent/controlplane.go](../../internal/agent/controlplane.go) (`AgentService`, `NewControlPlane`)
- CLI: [internal/agent/cli.go](../../internal/agent/cli.go) (`NewCmd`, `ClientFn`)
- Subdomain: [internal/org/member/](../../internal/org/member/) and [internal/org/invite/](../../internal/org/invite/)
- Wiring: [internal/boot/services.go](../../internal/boot/services.go) (`buildServices`), [internal/boot/README.md](../../internal/boot/README.md) ("Add a domain")
- Layers: [docs/architecture/README.md](../architecture/README.md)
