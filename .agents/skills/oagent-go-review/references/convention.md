# Convention pass

The rules live in `AGENTS.md` and `docs/howto/`. This list is what to look for in a diff. Each line names the rule and how to see it break.

## Every change

- **Public ids.** Any identifier leaving the server is a `public_id`: response fields, MCP tool output, CLI output and flags, console API calls, webhook payloads, error params. Search the diff for `uuid.UUID` fields on proto messages, `.ID.String()` in output, and slugs outside URL paths. See `docs/howto/public-ids.md`.
- **Surface → controlplane.** A Connect handler, MCP tool or signed-in page that calls a `Service` directly is a finding. Only pre-sign-in pages, operator commands and workers call services.
- **Permission table.** A new RPC without an `internal/permission` entry fails `TestEveryRPCHasAPermission`. Check that the entry is the narrowest permission that works.
- **Reach.** A new RPC that takes an org, project or resource id has a reach test that proves another org's or project's row is invisible.
- **Validation.** Every request field has a protovalidate rule (`TestEveryRequestFieldHasARule`); shared rules for `public_id`, `name` and `email`. No hand-written duplicate of a proto rule in Go or in the console.
- **Unit of work.** A write and its event or outbox row in one `db.Transactor` call. A write followed by a separate publish is a finding.
- **Error codes.** New codes appended to `apperror/codes.json` and `issued_codes.txt`, translated in every console locale, never renamed or reused.
- **Drivers and connections.** A feature (`internal/kernel/<feature>`) reaches Postgres, Redis or NATS only through its driver and a driver-agnostic interface (`db.Pool`/`db.Transactor`, `queue.Queue`, `ratelimit.Limiter`); the connection is a root package (`pg`, `redis`, `nats`) that returns the real client without wrapping it, opened by boot only when a feature selects it (`PostgresFeatures`, `RedisFeatures`, `NATSFeatures`); its errors and logs never carry credentials, and it is closed and in `/readyz`. Kernel features and connections take their own `Config` (`config.go`), never `config.Config`. A domain that imports pgx, `pg`, `nats` or `redis`, or a new import of pgx, go-redis or NATS outside the connection, its drivers, boot and the test helpers, is a finding even if depguard missed it.
- **Layering.** Imports follow `cmd → internal/cli → internal/boot → internal/controlplane → domains → internal/kernel/*`; domains never import `internal/cli`, `internal/boot`, `internal/controlplane` or `internal/web` (only `internal/web/layout`). depguard covers part of this.
- **READMEs.** A schema change updates `schema/README.md`; a domain, surface, worker or boot-order change updates `internal/boot/README.md`. Same change, current state only.
- **Config.** A new key is optional with a safe default unless it truly must be set; `.env.example` and `config.example.yml` stay minimal; `make config-examples` was run.
- **Comments.** Default none. Flag rationale, history, section headers and docs on unexported symbols. A comment that describes something the code does not do is worse than none.

## Things that pass review while wrong

- **A test that cannot fail.** Mentally delete the guard (permission entry, reach filter, validation rule): does a test go red? If not, the guard has no test.
- **A fake that enforces the rule under test.** A store fake that filters by org makes a reach test pass without the real filter.
- **A write split in two statements.** Both halves are guarded, but a second writer can interleave between them.
- **A success message for work that did not happen.** Check that every reported side effect has a matching write.
- **Timestamps.** Postgres `timestamptz` keeps microseconds; compare with `Truncate(time.Microsecond)` in integration tests, or they pass on macOS and fail on Linux CI.
- **Generated code edited by hand.** `gen/`, `ui/console/gen/`, `*_templ.go` change only through `make generate`.

## Domain module checklist

When the diff adds or reshapes `internal/<domain>/`, compare it with `docs/howto/module.md`:

- files present only when needed, each in its role (aggregate without I/O, `ports.go` for consumed interfaces, store split into reader and writer)
- constructors return the concrete type; interfaces live with the consumer
- surfaces: `controlplane.go`, `cli.go` (`NewCmd` taking `ClientFn`-style functions), `web.go` only for pre-sign-in pages
- wiring in `internal/boot` and its README; coverage at least 85% for the package
