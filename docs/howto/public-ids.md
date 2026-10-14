# Public ids

Use this whenever an identifier crosses a surface: a new RPC field, MCP tool argument, CLI flag or output, console API call, webhook payload or header, or a user-facing error. It also applies to a new aggregate that a surface will name. The rule is short and has no exceptions beyond the two listed below; the tests in [Review](#review) fail the build when a UUID leaks.

**The rule.** Every identifier on a surface is the `public_id`. UUIDs are internal only: primary keys, joins, ports, services, stores and the session `Principal`. Slugs appear only in URL paths; they are never a lookup key on the API, MCP or CLI (setting a slug on create or update is fine).

## Plan

- [ ] **Does the object reach a surface?** Then its table needs a `public_id` column, and its aggregate a `PublicID` field.
- [ ] **Wire name.** The field is `id` on the object itself, and `org_id`, `project_id`, `user_id`, `invite_id`, `delivery_id`… when it points elsewhere. It carries the public id. There is no separate `public_id` field on the wire.
- [ ] **Exceptions.** Members are addressed by the **user's** public id within the org; the membership's own `public_id` stays internal. API keys have **no** public id: the key `hint` (last 4 characters, unique per org among live keys) names a key.
- [ ] **Resolution.** Which handler turns the incoming public id into a UUID, and through which port? Every outgoing object needs the public ids of the org and project it points at.
- [ ] **No oracle.** An unknown id, a UUID, garbage, and another org's id must look the same to the caller.

## Build

1. **Column.** In the migration ([migration.md](migration.md)): `public_id TEXT NOT NULL UNIQUE`. Add it to the `schema/entity` binding and `schema/README.md`.
2. **Mint** in the aggregate constructor with `nanoid.New(21)`: 21 characters of an alphabet without look-alike characters (no `0`, `1`, `i`, `l`, `o`). No prefix. It never changes.

   ```go
   publicID, err := nanoid.New(21)
   if err != nil {
       return nil, err
   }
   ```

3. **Lookup.** `ByPublicID(ctx, publicID)` on the store and the service; an unknown id is the domain's `*NotFoundError` ([store-method.md](store-method.md)).
4. **Proto rule.** Every id field gets the shared shape rule, so a UUID or garbage is refused with `InvalidArgument` before the handler runs. Mark a removed field `reserved "public_id";`. See [rpc.md](rpc.md).

   ```proto
   string project_id = 2 [
     (buf.validate.field).required = true,
     (buf.validate.field).string.(oagent.v1.public_id) = true
   ];
   ```

5. **Resolve once, at the edge.** In `<domain>/controlplane.go`, through the `Refs` port in `ports.go`. Boot fills it with `publicIDs` (`internal/boot/publicids.go`), which serves `controlplane.Refs`. `ok == false` means unknown. Answer "not found" for unknown and out-of-reach alike:

   ```go
   // SECURITY: an unknown agent and an agent outside the caller's reach answer the same not-found.
   func (s *AgentService) reachable(ctx context.Context, publicID string) (*Agent, error) {
       a, err := s.agents.ByPublicID(ctx, publicID)
       if IsNotFoundError(err) {
           return nil, &NotFoundError{}
       }
       if err != nil {
           return nil, err
       }
       if !authz.ReachResource(ctx, a.OrgID, a.ProjectID, a.ID) {
           return nil, &NotFoundError{}
       }
       return a, nil
   }
   ```

   The service below it takes UUIDs only ([service-method.md](service-method.md)).

6. **Map outgoing objects** to public ids: `agentToProto(a, orgPublicID, projectPublicID)` sets `Id: a.PublicID`. Look the others up with `refs.OrgPublicID` / `refs.ProjectPublicID`.
7. **New kind of resolvable id?** Add a method to the domain's `Refs` port, implement it on `publicIDs` in `internal/boot/publicids.go`, add it to `controlplane.Refs`, and to `fakes.PublicIDs` for unit tests.
8. **Other surfaces.** CLI flags keep their names (`--org-id`, `--project-id`) and say "public id" in the help ([cli-command.md](cli-command.md)). MCP tools follow the proto ([mcp-tool.md](mcp-tool.md)). Event payloads use `a.PublicID` ([webhook-event.md](webhook-event.md)). The console sends public ids in API calls and uses slugs in routes (`/o/[org]/projects/[project]/agents/[agent]`); an object with no slug, like a webhook endpoint, uses its public id in the path ([console-page.md](console-page.md)).
9. **Errors.** A `ToAppError` meta never carries a UUID ([error-code.md](error-code.md)).
10. **Gates.** `make generate`, `make check`, `make test-integration`, `make cover`.

## Review

- [ ] No UUID in any response, tool result, CLI output (text or JSON), webhook body or header, or error meta.
- [ ] No slug used as a lookup key outside URL paths.
- [ ] Every id field in the proto has `(oagent.v1.public_id) = true`.
- [ ] Each incoming id is resolved once, in `controlplane.go`; services and stores take UUIDs.
- [ ] Unknown, foreign, UUID and garbage ids give the same answer (not found, or `InvalidArgument` for a bad shape).
- [ ] A new RPC is in `permission.Table()`, so the boot reach tests call it. A new service's `id` field is mapped in `reachIDs.serviceID` and seeded in `seedReach` (`internal/boot/reach_integration_test.go`).
- [ ] A new CLI command is in `TestCLI_JSONHoldsNoUUID`, with arguments that return at least one row.
- [ ] A filter RPC: `fillProbe` fills every `name` field with `"Probe"`, which matches no seeded row, so `TestPublicIDs_NoRPCAnswersWithAUUID` sees an empty answer. Prove a non-empty answer holds public ids in the handler tests and the CLI case above.

The tests that guard the rule:

| Test                                                    | Guards                                                                     |
| ------------------------------------------------------- | -------------------------------------------------------------------------- |
| `TestPublicIDs_NoRPCAnswersWithAUUID` (`internal/boot`) | every RPC response and error, called by a caller who can see the data      |
| `TestReach_UUIDsAndGarbageAreRefused` (`internal/boot`) | UUID and garbage ids → `InvalidArgument`; unknown ids → `NotFound`         |
| `TestReach_AnotherOrgIsRefused` (`internal/boot`)       | another org's public ids are refused and change nothing                    |
| `TestCLI_JSONHoldsNoUUID` (`internal/boot`)             | CLI JSON output                                                            |
| `TestMCP_ToolResultsHoldNoUUID` (`internal/boot`)       | every MCP tool result                                                      |
| `TestDeliveryCarriesPublicIDsOnly` (`internal/webhook`) | webhook body and headers                                                   |
| `TestAgentReach_IDOnlyRPCs` (`internal/agent`)          | a foreign agent, by public id or UUID, is not found (per-domain unit test) |
| `TestPostgres_ByPublicID` (`internal/agent`)            | the store finds by public id, and a UUID is not a public id                |

## Reference code

- Generator: [nanoid/nanoid.go](../../nanoid/nanoid.go) (`New`, `IsValid`)
- Proto rule: [api/oagent/v1/rules.proto](../../api/oagent/v1/rules.proto) (`public_id`), used in [api/agent/v1/agent.proto](../../api/agent/v1/agent.proto)
- Aggregate: [internal/agent/agent.go](../../internal/agent/agent.go) (`New`, `agentToProto`)
- Port: [internal/agent/ports.go](../../internal/agent/ports.go) (`Refs`)
- Edge: [internal/agent/controlplane.go](../../internal/agent/controlplane.go) (`reachable`, `project`, `toProto`)
- Boot resolver: [internal/boot/publicids.go](../../internal/boot/publicids.go) (`publicIDs`, `ResourceID`)
- Fake: [internal/testutil/fakes/publicids.go](../../internal/testutil/fakes/publicids.go) (`PublicIDs`)
- Tests: [internal/boot/publicid_integration_test.go](../../internal/boot/publicid_integration_test.go), [internal/boot/reach_integration_test.go](../../internal/boot/reach_integration_test.go), [internal/boot/cli_publicid_integration_test.go](../../internal/boot/cli_publicid_integration_test.go), [internal/boot/mcp_integration_test.go](../../internal/boot/mcp_integration_test.go), [internal/webhook/delivery_publicid_test.go](../../internal/webhook/delivery_publicid_test.go)
