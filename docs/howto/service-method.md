# Add a service method

Use this when a domain that already exists needs a new use case: create, change, delete, or a read that surfaces call. The `Service` in `<domain>/service.go` is the driving port. It enforces domain rules, owns the unit of work and works on UUIDs. It does not check permissions or reach: the `controlplane.go` handler did that before it calls in. For a new domain, start with [module.md](module.md). For a new database verb, see [store-method.md](store-method.md).

## Plan

- [ ] **Where does the rule live?** An invariant on one aggregate goes on the aggregate (`Agent.Update`, `agent.New`), not in the service. The service loads, calls the verb, saves.
- [ ] **Inputs.** UUIDs only (`orgID`, `projectID`, `id`). Public ids are resolved at the edge ([public-ids.md](public-ids.md)). More than two or three fields → a `<Verb>Request` struct, like `agent.CreateRequest`.
- [ ] **Writes.** One row → call the store. More than one row, or a row plus an event or a mail → one unit of work through `s.uow.RunInTx`.
- [ ] **Other domains.** Need something from another domain? Add a port in `ports.go` and a `With<Port>` option; boot wires it.
- [ ] **Failures.** List the expected failures. Each is a typed error the aggregate or store returns, plus an error code ([error-code.md](error-code.md)). Everything else is unexpected.
- [ ] **Callers.** Which surface calls it? A signed-in surface calls it from the domain's `controlplane.go`, never directly ([rpc.md](rpc.md)).

## Build

1. **Aggregate verb**, if the method mutates state, in `internal/<domain>/<domain>.go`. It validates and returns typed errors, no I/O:

   ```go
   func (a *Agent) Update(name, description string) error {
       n := strings.TrimSpace(name)
       if err := ValidateName(n); err != nil {
           return err
       }
       a.Name = n
       return nil
   }
   ```

2. **Failing test first** in `internal/<domain>/service_test.go` (package `<domain>_test`), over the fake store and the fake transactor:

   ```go
   svc := agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, testUnexpected())
   ```

3. **The method** in `internal/<domain>/service.go`. A one-sentence doc comment. Load, mutate, save, and send every store error through `errOrReport`:

   ```go
   // Update changes an agent's name and description.
   func (s *Service) Update(ctx context.Context, id uuid.UUID, name, description string) (*Agent, error) {
       a, err := s.store.ByID(ctx, id)
       if err != nil {
           return nil, s.errOrReport(ctx, "agent: update", err)
       }
       if err := a.Update(name, description); err != nil {
           return nil, err
       }
       if err := s.saveAndPublish(ctx, events.AgentUpdated, a); err != nil {
           return nil, s.errOrReport(ctx, "agent: update", err)
       }
       return a, nil
   }
   ```

4. **Unit of work** when it writes more than once. Use the `ctx` the closure receives, so every store call joins the transaction. A nested `RunInTx` joins the outer one:

   ```go
   // NOTE: one unit of work, so the save and its event commit together or not at all.
   func (s *Service) saveAndPublish(ctx context.Context, t events.Type, a *Agent) error {
       return s.uow.RunInTx(ctx, func(ctx context.Context) error {
           if err := s.store.Save(ctx, a); err != nil {
               return err
           }
           return s.publish(ctx, t, a)
       })
   }
   ```

5. **Required ids and cross-domain checks.** A missing UUID is `apperror.Invalid("<field>", apperror.FieldRequired)`. A project that must belong to the org goes through `tenant.RequireProjectInOrg(ctx, s.projects, orgID, projectID)`.
6. **Generated slugs.** An empty slug is minted with `slugpkg.Retry(slugpkg.MaxAttempts, IsAlreadyExistsError, ...)`, as in `agent.Service.createGenerated`.
7. **New store verb?** [store-method.md](store-method.md). **New error?** [error-code.md](error-code.md), and add its `Is…` helper to `IsExpectedError` in `errors.go`.
8. **Integration test** in `service_integration_test.go` (`//go:build integration`) when the method uses a unit of work: make the second write fail and assert nothing was saved.
9. **Surface.** Call it from `controlplane.go` ([rpc.md](rpc.md)), `cli.go` via the RPC ([cli-command.md](cli-command.md)), or a worker ([worker.md](worker.md)).
10. **Gates.** `make check`, `make comment-check`, `make test-integration`, `make cover` (at least 85% for the package). Update `internal/boot/README.md` if the wiring changed (a new port or `With…` option).

## Review

- [ ] Invariants sit on the aggregate; the service only orchestrates.
- [ ] The method takes UUIDs, never public ids or slugs as lookup keys.
- [ ] No permission or reach check in the service; that lives in `controlplane.go`. Domain rules (like `member.Authorize`) are fine.
- [ ] Expected errors return unchanged; every store error goes through `errOrReport`, so unknown failures reach `s.unexpected` with a `"<domain>: <verb>"` message.
- [ ] Two or more writes run in one `s.uow.RunInTx`, and the closure uses its own `ctx`.
- [ ] `service.go` imports no `database/sql` or `net/http` (lint rule `application-purity`).
- [ ] Another domain is reached only through a port; the service never imports it.
- [ ] Unit tests use `fakes.<Domain>Store` and `fakes.Transactor{}`; atomicity is proven on Postgres.
- [ ] Event payloads carry public ids only ([webhook-event.md](webhook-event.md)).

## Reference code

- Load, mutate, save: [internal/agent/service.go](../../internal/agent/service.go) (`Service.Update`)
- Create with request struct and generated slug: [internal/agent/service.go](../../internal/agent/service.go) (`Service.Create`, `createGenerated`)
- Unit of work with an event: [internal/agent/service.go](../../internal/agent/service.go) (`saveAndPublish`)
- Unit of work with a hook: [internal/org/service.go](../../internal/org/service.go) (`Service.Create`, `WithAfterCreate`)
- Ports and options: [internal/org/member/service.go](../../internal/org/member/service.go) (`WithEffects`, `WithDirectory`)
- Transactor: [internal/kernel/db/uow.go](../../internal/kernel/db/uow.go) (`Transactor`, `RunInTx`)
- Project in org: [internal/kernel/tenant/project.go](../../internal/kernel/tenant/project.go) (`RequireProjectInOrg`)
- Unexpected errors: [apperror/reporter.go](../../apperror/reporter.go) (`UnexpectedFunc`)
- Tests: [internal/agent/service_test.go](../../internal/agent/service_test.go), [internal/agent/service_integration_test.go](../../internal/agent/service_integration_test.go) (`TestService_CreateWithAFailedEventSavesNothing`)
