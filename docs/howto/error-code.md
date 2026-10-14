# Add an error code

Use this when a new failure mode must reach a person or a client as a code they can quote and the console can translate. Every error a surface shows carries a code `<DOM><NNN>` from one catalog, `apperror/codes.json`. The kind of the code picks the Connect status, the MCP payload and the CLI exit code.

Most bad input does **not** need a new code. A request that breaks its proto rules is refused before the handler with `GEN004` (`Validation`), the first field in `meta.field` / `meta.reason` / `meta.param`, and every field in `violations`. Add a rule ([rpc.md](rpc.md)), not a code, for shape: required, length, format, allowed values, item counts.

## Plan

- [ ] Is it a domain rule the proto cannot express (a uniqueness clash, a reserved slug, a state that forbids the action, a limit read from the database)? Then it needs a code. Shape alone is `GEN004`.
- [ ] Does an existing code already fit, with a new `reason`? Prefer a reason over a new code (`KEY003` carries `unknown` and `level`).
- [ ] Domain mnemonic: reuse the domain's three letters (`GEN`, `ORG`, `PRJ`, `AGT`, `KEY`, `WHK`, `MBR`, `INV`, `AUT`, `USR`, `INS`). A new mnemonic only for a new domain.
- [ ] Number: the next free one in that domain. Never reuse a retired number. `900`-`999` is reserved for unexpected failures (`GEN900`).
- [ ] Kind: `invalid`, `not_found`, `already_exists`, `unauthenticated`, `forbidden`, `failed_precondition`, `resource_exhausted` or `internal`.
- [ ] Params: the values the message needs (`slug`, `min`, `max`, `scope`), snake_case. Never a secret, a password or an email; user input only through `apperror.Echo`. `reason` is reserved for reasons.
- [ ] Not found and out of reach must answer the same code; never a code that tells them apart.

## Build

1. **Catalog entry.** Add one line to the domain's group in `apperror/codes.json`:

   ```json
   { "code": "AGT002", "name": "AgentAlreadyExists", "kind": "already_exists", "params": ["slug"], "reasons": [] }
   ```

   `name` becomes the Go constant `Code<Name>`; it is a Go identifier and unique.

2. **Issued list.** Append `AGT002 AgentAlreadyExists` as the **last** line of `apperror/issued_codes.txt`. The file is append-only: `TestCatalog_KeepsEveryIssuedCode` fails when a code is missing from it, removed from the catalog, or renamed.

3. **`make generate`.** `gen-apperror` rewrites `apperror/codes.gen.go` (the constants and the kind table) and refuses a bad code, a duplicate, an unknown kind, or a param or reason that is not snake_case.

4. **Domain error.** In `internal/<domain>/errors.go`, a typed error with an `IsXxxError` helper and a `ToAppError()` that sets the code, an English message, the declared params and the cause:

   ```go
   // ToAppError maps AlreadyExistsError to the canonical AlreadyExists envelope.
   func (e *AlreadyExistsError) ToAppError() *apperror.AppError {
       return apperror.New(apperror.CodeAgentAlreadyExists, "Agent slug already taken").
           WithMeta(map[string]string{"slug": apperror.Echo(e.Slug)}).WithCause(e)
   }
   ```

   A code with reasons sends `meta["reason"]` from a typed constant (`SlugLength`, `SlugFormat`). The service returns the domain error; `controlplane.Translate` (`/api`), `mcpinternal.ErrorPayload` (MCP) and `ExitCodeFor` (CLI) read `ToAppError()`. Handlers never build `apperror.New` for a domain failure.

5. **Translations.** Add the keys to **every** locale in `ui/console/i18n/locales/` (`en-US.json`, `id-ID.json`): `apperror.<CODE>`, plus `apperror.<CODE>_<reason>` for each reason. Messages use only the declared params:

   ```json
   {
     "apperror.AGT002": "An agent with the slug {slug} already exists"
   }
   ```

   Indonesian is natural Indonesian, not word for word.

6. **Wire sample.** Add the error to `samples()` in `internal/controlplane/catalog_test.go`, enough instances to send every param and every reason the entry declares.

7. **Field labels.** A handler-level `apperror.Invalid("<field>", reason)` (still `GEN004`) needs a `field.<field>` label in every locale; `TestEveryInvalidFieldHasALabel` checks it. Proto rule violations use the same `field.<name>` labels when present.

8. **Gates.** `make generate` (no diff after), `make check`, `make cover`, `make ui-test`.

## Pinning tests

| Test                                                  | Fails on                                                                                             |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `TestCatalog_EveryCodeHasItsKind` (`apperror`)        | a code with no kind, or a stale `codes.gen.go`                                                       |
| `TestCatalog_KeepsEveryIssuedCode` (`apperror`)       | a code not in `issued_codes.txt`, removed or renamed                                                 |
| `TestEveryCodeSendsItsCatalogParams` (`controlplane`) | a code with no sample, an undeclared meta key, a declared param or reason never sent                 |
| `TestEchoedInputIsCapped` (`controlplane`)            | echoed input longer than `apperror.EchoLimit`                                                        |
| `TestEveryInvalidFieldHasALabel` (`controlplane`)     | an `apperror.Invalid` field with no `field.<name>` label                                             |
| `catalog.test.ts` (`ui/console/app/lib/apperror`)     | a missing `apperror.<CODE>` or `_<reason>` key, an undeclared `{param}`, a stray key, locales differ |

## Review

- [ ] Shape errors are proto rules (`GEN004`), not new codes.
- [ ] `codes.json`, `issued_codes.txt` (appended at the end), `codes.gen.go` and every locale change in the same commit.
- [ ] No code was deleted, renamed, renumbered or reused.
- [ ] The kind matches the failure; the CLI exit code and Connect status follow from it.
- [ ] Params hold no secret, password or email; user input is passed through `apperror.Echo`.
- [ ] The error is constructed in the domain's `errors.go` through `ToAppError()`, never inline in a handler.
- [ ] Unknown and out-of-reach still answer the same code.
- [ ] `samples()` sends every param and reason.

## Reference code

- [apperror/codes.json](../../apperror/codes.json) and [apperror/issued_codes.txt](../../apperror/issued_codes.txt)
- [apperror/apperror.go](../../apperror/apperror.go): `New`, `WithMeta`, `WithCause`, `Invalid`, `Validation`, `Violation`
- [apperror/kind.go](../../apperror/kind.go): `Kind`, `KindOf`
- [apperror/truncate.go](../../apperror/truncate.go): `Echo`, `EchoLimit`
- [cmd/gen-apperror/main.go](../../cmd/gen-apperror/main.go): `parse`
- [internal/agent/errors.go](../../internal/agent/errors.go): `AlreadyExistsError`, `InvalidSlugError`, `ToAppError`
- [internal/controlplane/codes.go](../../internal/controlplane/codes.go): `connectCodeOf`
- [internal/controlplane/validate.go](../../internal/controlplane/validate.go): `violationsOf`, `reasonOf`
- [internal/controlplane/catalog_test.go](../../internal/controlplane/catalog_test.go): `samples`
- [internal/cli/exit.go](../../internal/cli/exit.go): `ExitCodeFor`
- [internal/mcp/payload.go](../../internal/mcp/payload.go): `ErrorPayload`
- [ui/console/app/lib/apperror/catalog.test.ts](../../ui/console/app/lib/apperror/catalog.test.ts)
