# Change the runtime contract

Use this when Go and the runtime must agree on a new field or shape in `POST /turn`. The contract version is `runtime.ContractVersion` (`internal/kernel/runtime/contract.go`); the runtime lists the versions it accepts in `apps/runtime/src/contract/request.ts`.

## Versions

| Version | Adds                                                                                                                                                           |
| ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1       | the turn, history, stored results, MCP servers with `allowedTools`                                                                                             |
| 2       | images in the input                                                                                                                                            |
| 3       | approvals: each MCP server sends `tools`, every tool with its `approval` spec (`never`, `always`, or `conditional` with `freeWhen`), instead of `allowedTools` |

## Optional fields

Version 3 also takes these optional fields. Go sends each optional field only when needed, and the runtime learns it first. A turn without them is unchanged:

- `knowledge`: passages retrieved for the turn, at most 10.
- `agent.helperModel`: the helper model, same shape as `agent.model`. Accepted and validated; nothing sends it yet.
- `limits.budget.helperPrices`: the helper's prices, same shape as `prices`, required when both `agent.helperModel` and a budget are sent. Accepted and validated; nothing sends it yet.
- `guardrails.keyPrefix`: the deployment's API key prefix. Accepted and validated; nothing sends it yet.
- Terminal `usage.helper`: the helper's tokens, same classes as `usage`. Sent only when they are above zero.

Go decodes the terminal strictly, so a Go release must learn a terminal field before any runtime emits it. The runtime emits a new terminal field only for a request that carried a field only a newer Go sends.

## Plan

- [ ] **Is a new version needed?** Not for an optional field: Go sends it only when needed, and the runtime learns it first, because the strict runtime refuses any field it does not know. A field every request must carry, or a changed shape, needs a new version.
- [ ] **Which side changes first?** Always the runtime: it learns the new version before Go sends it.
- [ ] **What does an old peer do?** The old runtime refuses the new version with 400. Plan the deploy so that never happens in production.

## Rollout order

Go and the runtime deploy separately, so a version change ships in three releases:

1. **Deploy the runtime** that accepts the old and the new version. For version 3 it must also send the approval interrupt reason fields `toolUseId`, `server` and `tool`; Go reads only those.
2. **Deploy Go** that sends the new version.
3. **Drop the old versions** from the runtime in the next release.

An old runtime refuses an unknown version with 400, and Go fails the run with `RUN006`. It refuses **every** request of the new version, so **every run fails**, not only the ones that use the new field. Rolling the runtime back past step 1 while the new Go is live causes the same outage. For version 3 no gated tool ever runs unguarded, but nothing runs at all until the runtime is back on a version that accepts v3.

## Build

1. The Go wire types and `ContractVersion` in `internal/kernel/runtime/contract.go`.
2. Bump every request golden in `internal/kernel/runtime/testdata/turn/` to the new version and shape; `TestGoldens_DecodeEveryCase` checks each one carries `ContractVersion`.
3. The runtime's validator and its HTTP test accept the old and new versions during the rollout.
4. A Go test with a fake old runtime that refuses the new version: the run fails with `RUN006` (`TestTurn_OldRuntimeV3RefusedFailsRun`).

## Review

- [ ] The runtime release that accepts the new version is deployed before the Go release that sends it.
- [ ] No shape of the new version is sent under the old version number.
- [ ] Never encode `null` for a required runtime field: send `[]` or `{}`, and test the JSON Go actually sends (`TestTurn_AbsentOnlyFreeWhenSendsEmptyValues`).
