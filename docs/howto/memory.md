# Agent memory: run it, and add a driver

Use this to run agent memory for a deployment, or to add a memory driver. A project keeps facts in named memory stores; an agent whose config names a store remembers short facts about each person it talks to and recalls them on that person's next turn. Facts belong to one end user of one agent in one store of one project; nothing is shared between people, agents or stores.

## How it works

1. **Stores.** `MemoryService` creates, lists, gets and deletes a project's stores (`memory_store_*` tools, `oagent memory store`). A store names its backend (`native` today) and whether it remembers named API users (`remember_api_users`, off by default, set at create; `--remember-api-users`). A new store needs its backend enabled (`MEM003 backend_disabled`); names are unique per project (`MEM005`).
2. **Opt-in per agent.** `AgentConfig.memory_store_id` (the console's store select) is empty by default, so memory is off. Saving or publishing a store of another project, or one gone, is refused with `AGT007 memory_store`; one whose backend is off with `AGT007 memory_store_disabled`. Publishing share-locks the store. A set store counts as untrusted input for the publish rule, so the agent cannot also hold a tool that runs without approval (`AGT007 unapproved_tool`).
3. **Who is remembered.** A turn remembers only when every user message it takes has one known speaker and, on an API thread, the speaker is the thread's bound end user. A WhatsApp number or LID, a signed widget user and a `person:` API user (a member's personal key) are always remembered. A named `Oagent-User` is remembered only on a store with `remember_api_users`. Groups, Try it drafts, `key:` actors and anonymous widget visitors are never remembered. Resume and continue legs never recall or extract.
4. **The store decides.** Once per turn, run asks the memory service how the version's store takes part: whether it extracts facts itself and whether it remembers named API users. The service checks that the store is in the run's project and its backend is enabled, then answers from that backend's driver. A gone store or a disabled backend gives the turn no memory (fail closed).
5. **Recall.** Before the turn, run asks the store for at most 5 facts matching the conversation text (newest first when nothing matches) and sends them in the turn request's `memory.facts`. A failed recall sends none; the turn still runs.
6. **Extract.** The request's `memory.extract` is `facts` (the runtime's helper model extracts facts), `userTexts` (the runtime returns the person's masked texts for a driver that extracts itself) or `none` (recall only, for example a priced run whose helper model has no prices). The runtime masks phone numbers, emails, cards and keys first.
7. **Record.** After the run settles (completed, limit reached or awaiting approval), Go checks the caps again (10 facts of at most 500 characters, or 10 texts of at most 4000) and records them in the leg's store, on its own context with a 5 s timeout. A failure is logged and counted (`memory.record_failed`) and never fails the run.
8. **Admins.** `ListMemories`, `DeleteMemory` and `ClearMemories` (`memory_list`, `memory_delete`, `memory_clear`, `oagent memory list|delete|clear --store`) list, delete and clear a person's facts in one store. Members hold `memory:read`; admins hold `memory:write` too. These work with the backend off.
9. **Deleting a store.** `DeleteMemoryStore` is refused with `MEM004` while a latest published agent version uses the store. Otherwise it erases the store's facts through the store's own driver (`Facts.ClearStore`), then deletes the row, in one unit of work.

## Run it (operator)

| Setting                                 | Effect                                                                                                                 |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `memory.native.enabled: true` (default) | `native` stores keep facts in Postgres (`memory_facts`, migration 36) and recall them with full-text search            |
| `memory.native.enabled: false`          | no `native` store can be created and runs on one never recall or record; admins can still list, delete and clear facts |

Each person keeps at most 1000 facts per agent in a store; the oldest go first. Deleting an end user, an agent, a store or a project deletes their facts. A recycled WhatsApp number inherits the previous owner's facts: clear them from the end-user page.

## Add a driver

A driver is one `Backend` value, one package, one enabled flag, one migration that widens the backend check and one boot entry. Run, API, MCP, CLI and console need no change.

- [ ] `internal/memory/<driver>/` implements `memory.Facts` and passes `memorytest.Run` with its own `Seed`.
- [ ] One `Backend` constant in `internal/memory/memory.go`, and its value in the proto's `MemoryBackend` enum with `backendToProto` and `backendOf` in `internal/memory/controlplane.go`.
- [ ] One `memory.<driver>.enabled` flag (plus the driver's own keys) in `config/memory.go`, listed by `Backends()`, then `make config-examples`.
- [ ] One migration that widens `memory_stores_backend_check` to the new value (see [migration](migration.md)), and the `schema/README.md` row.
- [ ] One entry in `memoryDrivers` (`internal/boot/memory.go`), built only when the flag is on.
- [ ] The shared AWS connection, if the driver needs it and it is not there yet.

### Plan

- [ ] `Extracts()` says whether the driver extracts facts from user texts itself; true makes every leg on its stores send `userTexts` and pins no helper prices.
- [ ] Isolation is the contract: a fact is visible only to the scope that recorded it. Never key a fact on a thread, a phone number, a name or any value a client sends.
- [ ] The driver's fact ids and paging tokens are opaque to oagent: ids match `^[A-Za-z0-9_-]{1,128}$`, tokens are at most 4096 bytes.
- [ ] A driver whose flag goes off is no longer built, so its stores answer `MEM006` and their facts cannot be erased until it is on again.
- [ ] `ClearStore` is synchronous and bounded by the caller's ctx: a store delete runs it under the store's `FOR UPDATE`, so publishes wait at most that long.
- [ ] `Record` runs under the store's share lock (the memory service takes it), so a store delete waits for it and no fact lands after the store's erase.
- [ ] The trust model for a store with `remember_api_users`: the server cannot tell who sits behind an `Oagent-User`, and any API key that runs an agent on the store can name any user and so read (through the agent's answers) and add that user's facts. Opt in only when every such key belongs to one integration. The API path needs no `memory:read`: the key's run scope is what reaches the facts.

### Build

1. **Driver.** Every method calls `scope.Check(needAgent)` (or `scope.CheckStore()` for `ClearStore`) before any I/O (a missing field is a `*memory.IncompleteScopeError`) and refuses a scope whose org is not the context's tenant (`*memory.TenantMismatchError`); both are bugs, reported, never answers. Recall and Record need the agent; List, Delete and Clear may leave it nil to span the person's agents in the store. Every read and write filters org, project, store, end user and, when set, agent. `Delete` of a fact outside the scope is a `*memory.NotFoundError`, the same answer as a missing one. A driver that does not extract cleans its input with `memory.CleanFacts` and stores a repeated fact once; one that does accepts `Turn.UserTexts`.
2. **Contract suite.** Run `memorytest.Run(t, newBackend)`; the backend's `Seed` writes facts directly. The suite seeds one fact in each of 32 scopes (2 orgs x 2 projects x 2 stores x 2 agents x 2 end users) and proves recall, list, delete, clear and clear-store never cross a scope, plus paging and odd queries. Record's own cases (caps, dedupe, pruning, source run) run for a driver that does not extract; one that does gets `RecordAcceptsUserTexts` and `RecordRefusesAnIncompleteScope`. `memorytest.NewFake(memorytest.Extracting())` is the reference for an extracting driver.
3. **Docs.** This page, `internal/boot/README.md`, `schema/README.md`, and the PR body's operator notes.

### Review

- [ ] `memorytest.Run` passes, and a mutation that drops any scope filter, the store filter included, fails it.
- [ ] No fact text, user text or query is logged; counts and public ids only.
- [ ] Erasure reaches the driver: deleting a store calls `ClearStore`, and a later `DeleteEndUser` must call `Clear` with the person's scope in each store and no agent.

## Reference code

- [internal/memory/memory.go](../../internal/memory/memory.go): `Backend`, `MemoryStore`, `Scope`, caps, `CleanFacts`, `Hash`
- [internal/memory/stores.go](../../internal/memory/stores.go): store create, list, get and delete
- [internal/memory/service.go](../../internal/memory/service.go): admin scope resolution and the run's dispatch by backend
- [internal/memory/native/native.go](../../internal/memory/native/native.go): the Postgres driver
- [internal/memory/memorytest/memorytest.go](../../internal/memory/memorytest/memorytest.go): the contract suite
- [internal/run/memory.go](../../internal/run/memory.go): eligibility, the store's mode, extract mode, recall and record
- [internal/boot/memory.go](../../internal/boot/memory.go): the drivers by backend
- [schema/migrations/036_memory_facts.sql](../../schema/migrations/036_memory_facts.sql)
