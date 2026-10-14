# Add an MCP tool

Use this to let an MCP host (Claude, an IDE, an agent) call an RPC. A tool and a Connect RPC are one call reached two ways: the tool is generated from an annotated rpc and calls the same `controlplane.go` handler. It has no code of its own. So the RPC comes first: [rpc.md](rpc.md).

Two transports serve the same registry: `/mcp` over HTTP (an API key or Basic auth, no cookie) and `oagent mcp` over stdio, which runs as the `session.System()` operator.

## Plan

- [ ] Does the RPC exist, with rules and a `permission.Table()` entry? If not, start with [rpc.md](rpc.md).
- [ ] Is it unary? The generator refuses streaming RPCs.
- [ ] Tool name: `<domain>_<verb>` in snake case (`agent_create`, `webhook_retry_failed`), matching `^[a-z_][a-z0-9_]{0,63}$`, unique across all protos.
- [ ] Does it write (`mutation: true`)? Does it delete or overwrite (`destructive: true`)? Both hints must be true to the facts; hosts use them for confirmation prompts.
- [ ] Should the model see it, or only the MCP view (`app_only: true`, needs `ui`)? A step that sends something on the user's behalf after a draft (`member_invite_confirm`) is app-only.
- [ ] Does it need a view in the MCP App (`ui: "app"`), or is the generic `DefaultView` enough?

## Build

1. **Annotate the rpc** in `api/<domain>/v1/<domain>.proto`:

   ```proto
   rpc DeleteAgent(DeleteAgentRequest) returns (DeleteAgentResponse) {
     option (mcp.v1.tool) = {
       name: "agent_delete"
       description: "Delete an agent by id."
       required: ["id"]
       mutation: true
       destructive: true
       ui: "app"
     };
   }
   ```

   - `description` is what the model reads; without it the rpc's leading comment is used, and with neither the generator fails.
   - `required` names fields by proto or JSON name; an unknown name fails the generator. It becomes the schema's `required` list. It is separate from the proto rules: a rule's `required` only adds `minLength: 1` to the field, and the validator refuses the call either way.
   - `title` overrides the title derived from the name (use it for acronyms).
   - Never add a permission or scope field: `api/mcp/v1/annotations.proto` forbids it. The tool takes its RPC's entry from `internal/permission`.

2. **`make generate`.** `cmd/protoc-gen-mcp` (code in `internal/tool/protocmcp`) writes `gen/go/<domain>/v1/<domain>v1mcp/<service>.mcp.go` with a `…ToolName` constant and `Register<X>ServiceTools`. The input schema carries the request rules (`minLength`, `maxLength`, `pattern`, `enum`, `format`, item counts). The generated handler decodes the JSON, calls `validate`, then the Connect handler.

3. **New service only: register it** in `buildMCPServer` (`internal/boot/mcp.go`), with the same control-plane constructor the API uses:

   ```go
   agentv1mcp.RegisterAgentServiceTools(registry, agent.NewControlPlane(svcs.Agents, refs), permission.For, validate)
   ```

   An existing service needs no wiring: the new tool is already inside its `Register…Tools`.

4. **View (optional).** Add a Vue component in `ui/mcp/src/views/` and map the tool name to it in `registry` (`ui/mcp/src/views/index.ts`). Pure helpers go in a `.ts` file with a `.test.ts` beside it (`members.ts`, `members.test.ts`). Then `make ui-mcp` rebuilds `internal/mcp/ui/assets/app.html`, which the binary embeds.

5. **Tests.**
   - Add the tool and its permissions to `want` in `TestMCPTools_PermissionsComeFromTheTable` (`internal/boot/mcp_test.go`).
   - An app-only tool gets a test like `TestMCPTools_InviteConfirmIsForTheAppOnly`: visible to the view with `mcp.appsUI` on, not registered with it off.
   - `TestMCP_ToolResultsHoldNoUUID` (`internal/boot/mcp_integration_test.go`) checks results hold public ids only.

6. **Docs.** When this is the domain's first tool, mark the "MCP tools" column of the domain table in `internal/boot/README.md`.

7. **Gates.** `make generate` (no diff after), `make check`, `make cover`; with view changes, `make ui-test` and `pnpm -r typecheck`.

## How it behaves

- `tools/list` and `tools/call` both check every permission of the tool against the caller's (`mcpinternal.ScopesFromContext`). A tool whose RPC has no table entry has no permissions and is denied, even to the stdio operator.
- A request that breaks its rules is refused with `GEN004` and the same `violations` as on `/api`. Bad JSON is `INVALID_ARGUMENT`; a missing permission is `FORBIDDEN`.
- A tool failure is a tool result with `isError`, never a JSON-RPC error. App errors carry their catalog code, message, meta and violations (`mcpinternal.ErrorPayload`); anything else is `GEN900`.
- With `mcp.appsUI` off, app-only tools are not registered and `ui` metadata is not sent.

## Review

- [ ] The tool is an annotation on an existing, unary rpc; no hand-written tool handler, no scope in the proto.
- [ ] The RPC is in `permission.Table()`; the tool's permissions in `TestMCPTools_PermissionsComeFromTheTable` match it.
- [ ] `TestMCP_KeyScopesTheToolList` (`internal/boot/mcp_integration_test.go`) lists the tools a key with that permission sees; add the new tool there.
- [ ] Name follows `<domain>_<verb>`; description says what it does and which ids it needs.
- [ ] `mutation` and `destructive` are accurate.
- [ ] `required` lists the fields a caller must send (the generator accepts only real field names).
- [ ] `app_only` is set on any step that must stay behind the user's click.
- [ ] Results hold public ids only.
- [ ] `internal/boot/README.md` still describes the domain's surfaces (a new tool on a domain that already has MCP tools needs no change; a domain's first tool does).

## Reference code

- [api/mcp/v1/annotations.proto](../../api/mcp/v1/annotations.proto): `Tool`
- [api/agent/v1/agent.proto](../../api/agent/v1/agent.proto): `AgentService` annotations
- [api/invite/v1/invite.proto](../../api/invite/v1/invite.proto): `member_invite_confirm` (app-only)
- [internal/tool/protocmcp/generator.go](../../internal/tool/protocmcp/generator.go): `newTool` (refusals)
- [internal/tool/protocmcp/rules.go](../../internal/tool/protocmcp/rules.go): `applyFieldRules`
- [gen/go/agent/v1/agentv1mcp/agent_service.mcp.go](../../gen/go/agent/v1/agentv1mcp/agent_service.mcp.go): `RegisterAgentServiceTools`
- [internal/boot/mcp.go](../../internal/boot/mcp.go): `buildMCPServer`
- [mcp/registry.go](../../mcp/registry.go): `ToolSpec`, `Registry`
- [mcp/server.go](../../mcp/server.go): app-only and `ui` handling
- [internal/mcp/payload.go](../../internal/mcp/payload.go): `ErrorPayload`
- [internal/mcp/auth.go](../../internal/mcp/auth.go): `Authenticate`, `ScopesFromContext`
- [ui/mcp/src/views/index.ts](../../ui/mcp/src/views/index.ts): `resolveView`
- [internal/boot/mcp_test.go](../../internal/boot/mcp_test.go): `TestMCPTools_PermissionsComeFromTheTable`
