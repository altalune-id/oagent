# Add an RPC

Use this when a signed-in client (the console, the CLI, an integrator's script, an MCP host) needs a new call. Every signed-in call is a Connect RPC under `/api`; MCP tools and CLI commands are thin clients of the same handler. Business logic goes in [service-method.md](service-method.md), the query in [store-method.md](store-method.md), a new domain in [module.md](module.md).

The request chain on `/api` is: authn and the permission check (`internal/permission`), the tenant from the principal, the proto rules (`GEN004`), then your handler. The handler turns public ids into ids, checks reach, calls the service and maps ids back.

## Plan

- [ ] A new RPC, or a new field on an existing one (for example an optional filter on a `List` call)? A field needs no new permission, tool or command. When the request does not say, ask.
- [ ] Which domain owns it? The handler goes in that domain's `controlplane.go`, never in `internal/controlplane`.
- [ ] Which permission does the caller need (`scope/scopes.json`)? Reuse `<domain>:read` / `<domain>:write`; `authz.AnyAuthenticated` only for calls about the caller itself, `authz.PersonOnly` when a key must never call it.
- [ ] Which ids does the request name? Every id is a public id ([public-ids.md](public-ids.md)). Slugs never travel as ids.
- [ ] Which reach check fits: `ReachOrg`, `ReachWholeOrg`, `ReachProject`, `ReachWholeProject` or `ReachResource` (`internal/kernel/authz`)?
- [ ] Which shape rules go in the proto, and which domain rules stay in the service? Shape (required, length, format) lives only in the proto; never repeat it in Go. The service checks what only it can know: existence, state, ownership, uniqueness.
- [ ] New failure modes? Each needs a code ([error-code.md](error-code.md)).
- [ ] Is it an MCP tool ([mcp-tool.md](mcp-tool.md)), a CLI command ([cli-command.md](cli-command.md)), a console page ([console-page.md](console-page.md))?
- [ ] Does it send a webhook event ([webhook-event.md](webhook-event.md)) or need a rate limit ([rate-limit.md](rate-limit.md))?

## Build

1. **Proto.** Add the `rpc` and its `XxxRequest` / `XxxResponse` to `api/<domain>/v1/<domain>.proto`. Put a rule on every request field. Use the shared rules from `api/oagent/v1/rules.proto` (`public_id`, `name`, `email`, `slug`) and pair them with `required` when the field must be set:

   ```proto
   message GetAgentRequest {
     string id = 1 [
       (buf.validate.field).required = true,
       (buf.validate.field).string.(oagent.v1.public_id) = true
     ];
   }
   ```

   When no shared rule fits, write a field CEL rule whose id is `<domain>.<field>.<rule>`, with the limit as a trailing number (`agent.description.max_len.2000`). The validator turns the last segment into the reason and the number into `param`. Add every new rule id to `internal/controlplane/testdata/rule_reasons.json`. A field that truly needs no rule (an empty request, a bool) goes in the `unruled` allowlist in `internal/controlplane/rules_test.go`, with a reason.

2. **`make generate`.** It runs `buf generate` (Go, Connect, OpenAPI under `gen/openapi`, TypeScript under `ui/console/gen`), `buf generate --template buf.gen.mcp.yaml` (MCP tools), `gen-scope` and `gen-apperror`. Never edit `gen/` by hand.

3. **Permission.** Add the procedure to `Table()` in `internal/permission/permission.go`, keyed by the generated `…Procedure` constant. Bump the count and the per-service tally in `TestEveryRPCHasAPermission`. A new permission is a `scope/scopes.json` entry (with its `labelKey` translated in every console locale) followed by `make generate`.

4. **Service method.** Add it in `internal/<domain>/service.go` ([service-method.md](service-method.md)). It takes ids, enforces domain rules and owns the transaction. It never checks permissions or reach.

5. **Handler.** Add the method to the domain's control-plane type in `internal/<domain>/controlplane.go`. Shape, copied from `AgentService.GetAgent`:

   ```go
   // GetAgent loads an agent by id.
   func (s *AgentService) GetAgent(
       ctx context.Context,
       req *connect.Request[agentv1.GetAgentRequest],
   ) (*connect.Response[agentv1.GetAgentResponse], error) {
       a, err := s.reachable(ctx, req.Msg.GetId())
       if err != nil {
           return nil, err
       }
       out, err := s.toProto(ctx, a)
       if err != nil {
           return nil, err
       }
       return connect.NewResponse(&agentv1.GetAgentResponse{Agent: out}), nil
   }
   ```

   - Turn each public id into an id once, through the domain's `Refs` port (`OrgID`, `ProjectID`, `ResourceID`, …). Map every outgoing id back (`OrgPublicID`, `ProjectPublicID`). No UUID leaves the handler.
   - Check reach with `internal/kernel/authz` right after the lookup. An unknown id and an out-of-reach id answer the **same** not-found error (mark it `// SECURITY:`).
   - A permission check that depends on the request (a key kind, org-wide events) runs here, after validation.
   - Do not repeat shape checks the proto rules already make. Return domain errors as they are; `controlplane.Translate` maps their `ToAppError()` onto the wire.

6. **New service only.** Wire it in every place that lists services:
   - `internal/controlplane/server.go`: `Services`, `Refs` (embed the domain's port), `Server` field, `New`, a `New<X>ServiceHandler` mount in `Handler`.
   - `internal/boot/http.go` (`buildAPIHandler`) and `internal/boot/publicids.go` when the `Refs` port grows.
   - `internal/apiclient/client.go`: a `Client` field and its constructor in `New`.
   - `ui/console/app/plugins/connect.client.ts`: a `createClient(...)` entry.
   - Test lists: `allProcedures` in `internal/permission/permission_test.go`, the imports in `internal/controlplane/rules_test.go`.

7. **MCP tool, if exposed.** Add the `(mcp.v1.tool)` option on the rpc ([mcp-tool.md](mcp-tool.md)).

8. **CLI command, if exposed.** Add it to the domain's `cli.go` ([cli-command.md](cli-command.md)).

9. **Console, if a person needs it there.** Most RPCs do; ask when it is unclear where it belongs. Store action in `ui/console/app/stores/<domain>.ts`, the page or modal, and i18n keys in every locale under `ui/console/i18n/locales/` ([console-page.md](console-page.md)).

10. **Tests.**
    - Handler: `internal/<domain>/controlplane_test.go`.
    - Reach: `internal/<domain>/reach_test.go` calls the handler as a member and as a key of another org, with the public id and the UUID, and asserts not-found and no write.
    - Boot integration (run by `make cover`): `internal/boot/reach_integration_test.go` and `publicid_integration_test.go` iterate over `permission.Table()`, so the new RPC is called automatically. Teach them any new id field (`reachIDs`, `seedReach`, the `tenantIDs` map, `fillProbe`/`probeStrings`, `snapshot` for new tables). An RPC that names no tenant goes in `noTenantInput`, one a project-limited key may call in `limitedKeyAllowed`, each with a reason.

11. **Docs.** A new service, MCP tool, CLI group or wiring change updates the domain table and diagrams in `internal/boot/README.md` in the same commit (current wiring only, no history).

12. **Gates.** `make generate` (no diff after), `make check`, `make comment-check`, `make cover`; with console or MCP UI changes, `make ui-test` and `pnpm -r typecheck`.

## Review

- [ ] Every request field has a rule or an `unruled` entry with a reason; shared rules are used where the limit matches.
- [ ] The procedure is in `permission.Table()`; nothing else decides who may call it.
- [ ] The handler lives in `internal/<domain>/controlplane.go`; the domain imports no `internal/controlplane`, `internal/boot`, `internal/cli` or `internal/web`.
- [ ] Public ids in, public ids out; no UUID or slug-as-id on the wire.
- [ ] Reach is checked; unknown and out-of-reach answer the same not-found.
- [ ] The service holds the business rules; the handler holds only id mapping, reach and request-dependent permission checks.
- [ ] Every new error code follows [error-code.md](error-code.md).
- [ ] The boot reach and public-id tests call the RPC and pass; any allowlist entry has a reason.
- [ ] `gen/` is regenerated, not hand-edited; `internal/boot/README.md` matches the wiring.

## Reference code

- [api/agent/v1/agent.proto](../../api/agent/v1/agent.proto): `AgentService`, `CreateAgentRequest`
- [api/oagent/v1/rules.proto](../../api/oagent/v1/rules.proto): `public_id`, `name`, `email`, `slug`
- [internal/agent/controlplane.go](../../internal/agent/controlplane.go): `AgentService`, `reachable`, `project`, `toProto`
- [internal/permission/permission.go](../../internal/permission/permission.go): `Table`, `For`
- [internal/kernel/authz/reach.go](../../internal/kernel/authz/reach.go): `ReachProject`, `ReachResource`
- [internal/controlplane/server.go](../../internal/controlplane/server.go): `Services`, `New`, `Handler`, `handlerOptions`
- [internal/controlplane/validate.go](../../internal/controlplane/validate.go): `Validator`, `reasonOf`
- [internal/controlplane/rules_test.go](../../internal/controlplane/rules_test.go): `TestEveryRequestFieldHasARule`, `unruled`
- [internal/agent/reach_test.go](../../internal/agent/reach_test.go): `TestAgentReach_IDOnlyRPCs`
- [internal/boot/reach_integration_test.go](../../internal/boot/reach_integration_test.go): `seedReach`, `noTenantInput`, `limitedKeyAllowed`
- [internal/boot/publicid_integration_test.go](../../internal/boot/publicid_integration_test.go): `TestPublicIDs_NoRPCAnswersWithAUUID`, `fillProbe`
- [internal/boot/publicids.go](../../internal/boot/publicids.go): `publicIDs`
