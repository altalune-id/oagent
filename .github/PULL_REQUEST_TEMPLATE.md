## Summary

<!-- What changed and why, in a few sentences. -->

## Type

<!-- Keep one; it matches the commit type in .gitmessage.txt. -->

feat | fix | chore | refactor | docs | test | perf | build | ci | style

## Verification

- [ ] `make check` passes (vet, every linter, unit tests)
- [ ] `make cover` passes if Go code changed (integration tests, 85% per package)
- [ ] `make ui-test` and `pnpm -r typecheck` pass if `ui/` changed
- [ ] `make generate` run and committed if a `.proto`, `.templ`, `scopes.json` or `codes.json` changed
- [ ] `make config-examples` run if a config struct tag changed
- [ ] `schema/README.md` updated if a migration changed a table, column, constraint or index
- [ ] `internal/boot/README.md` updated if domains, surfaces, workers or boot order changed
- [ ] New error codes: `apperror/codes.json`, `issued_codes.txt` and every console locale
- [ ] Surfaces show `public_id`s only (no UUIDs, slugs only in URL paths)
- [ ] Screenshots attached for UI changes

## Breaking changes

<!-- API, MCP, CLI, config or webhook payload changes, and the migration path. Delete if none. -->

## Notes for the reviewer

<!-- Anything not obvious from the diff. Delete if none. -->
