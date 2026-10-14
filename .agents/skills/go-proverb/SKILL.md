---
name: go-proverb
description: Use when writing, designing or reviewing Go code and judging whether a design is idiomatic — interfaces, concurrency, errors, reflection, unsafe, cgo, dependencies, zero values, gofmt, documentation, panics. Also use for "is this idiomatic Go", "go proverbs" or "rob pike".
---

# Go Proverbs

The Go Proverbs are short sayings about writing simple, clear Go. Rob Pike
presented them at Gopherfest 2015. The list comes from
https://go-proverbs.github.io/.

Use them as a lens, not a rulebook: when a design feels heavy, find the
proverb it breaks and say why. The explanations below are a summary in our own
words.

## The proverbs

### 1. Don't communicate by sharing memory, share memory by communicating.
Pass ownership of data between goroutines over channels instead of letting
many goroutines touch one variable under locks. When a value is sent, the
sender stops using it; only one goroutine owns it at a time.

In review, flag: several goroutines reading and writing the same map or struct with ad hoc locking.

### 2. Concurrency is not parallelism.
Concurrency is how you structure a program as independent parts. Parallelism
is running things at the same time on many cores. Good concurrent structure
may or may not run in parallel; do not add goroutines expecting speed.

In review, flag: goroutines added "for performance" with no measurement and no independent work to do.

### 3. Channels orchestrate; mutexes serialize.
Use channels to coordinate work and signal between goroutines. Use a mutex
to protect a small piece of shared state. Each tool fits a different job;
a channel used as a lock, or a mutex used to sequence a workflow, is a smell.

In review, flag: a buffered channel of size 1 used as a lock, or nested mutexes driving a multi-step flow.

### 4. The bigger the interface, the weaker the abstraction.
Small interfaces (one to three methods, like io.Reader) are easy to
implement, fake and combine. A large interface ties every caller to every
method and usually mirrors one concrete type.

In review, flag: an interface with many methods that only one type implements, defined next to that type.

### 5. Make the zero value useful.
Design types so that `var x T` works without a constructor, as with
bytes.Buffer and sync.Mutex. This removes setup steps and nil checks. When a
zero value cannot be valid, make that obvious with a constructor.

In review, flag: a type that panics or misbehaves when declared without `New...`, with no doc saying so.

### 6. interface{} says nothing.
An empty interface (`any`) carries no contract, so the compiler cannot help
you. Prefer concrete types, a small interface, or generics with a constraint.

In review, flag: `any` or `map[string]any` parameters where the set of real types is known.

### 7. Gofmt's style is no one's favorite, yet gofmt is everyone's favorite.
One enforced format ends style debates and makes all Go code look the same.
Accept the tool's output instead of fighting it.

In review, flag: unformatted files, or discussion of layout that gofmt already decides.

### 8. A little copying is better than a little dependency.
Each dependency adds build, security and upgrade cost. If you need a few
lines from a library, copy them (with credit and license) rather than import
the whole module. Do not take this to mean "copy large systems".

In review, flag: a new module in go.mod that is used for one small helper.

### 9. Syscall must always be guarded with build tags.
The syscall package differs by operating system. Code that calls it must sit
in files with build constraints so other platforms still compile.

In review, flag: syscall use in a file without a `//go:build` line or OS file suffix.

### 10. Cgo must always be guarded with build tags.
Cgo needs a C toolchain and breaks simple cross-compiling. Keep it behind
build tags and offer a pure Go path where you can.

In review, flag: `import "C"` in a file that every build must compile.

### 11. Cgo is not Go.
With cgo you lose memory safety, easy cross-compiling, fast builds and cheap
calls. Choose it only when there is no Go alternative and accept the cost.

In review, flag: cgo added for something a pure Go library already does.

### 12. With the unsafe package there are no guarantees.
`unsafe` skips the type system and the compatibility promise. Code that
works today may break on the next Go release or another architecture.

In review, flag: any `unsafe` import in service code, especially for "speed" without a benchmark.

### 13. Clear is better than clever.
Code is read far more than it is written. Plain loops, explicit names and
obvious control flow beat compact tricks that need a comment to decode.

In review, flag: a one-liner or generic helper that needs more than a moment to understand.

### 14. Reflection is never clear.
reflect code is hard to read, slow and fails at run time instead of compile
time. Use it inside well-tested libraries (encoding, validation), not in
everyday business logic.

In review, flag: `reflect` in a handler or service where a type switch or interface would do.

### 15. Errors are values.
An error is an ordinary value you can store, wrap, compare and program with.
Use that: typed errors, sentinel values, `errors.Is`/`errors.As`, and helpers
that reduce repeated checks.

In review, flag: matching errors by string content, or losing the cause by not wrapping with `%w`.

### 16. Don't just check errors, handle them gracefully.
`if err != nil { return err }` is not handling. Add context, decide whether
to retry, fall back or stop, and log once at the edge rather than at every
layer.

In review, flag: bare `return err` that loses where it came from, or the same error logged at each layer.

### 17. Design the architecture, name the components, document the details.
Good names carry the design: if the parts are named well, the structure
explains itself. Put written documentation on the details that names cannot
say.

In review, flag: vague package or type names such as `util`, `common`, `manager` or `helper`.

### 18. Documentation is for users.
Doc comments should tell the caller what a function does and how to use it,
not how it is built inside. Write for the person who will call the code.

In review, flag: godoc that restates the implementation instead of the contract.

### 19. Don't panic.
Source: https://go.dev/wiki/CodeReviewComments#dont-panic

Return errors for normal failure. Keep panic for truly impossible states
and programmer bugs, and do not let a panic cross a package boundary.

In review, flag: `panic` on bad input, missing config at run time or a failed request.

## Applying them in oagent

- **Small interfaces at the consumer (4).** Each domain declares the ports it
  needs in its own `ports.go` (for example `internal/agent/ports.go`). Keep
  each port to the methods that domain calls; do not export one big store
  interface for everyone.
- **Errors are values (15, 16).** Return the typed errors from `apperror`
  with an error code, wrap causes with `%w`, and map them to HTTP, RPC or MCP
  responses once at the edge. Never branch on error strings.
- **Share memory by communicating (1, 3).** Workers, consumers and the outbox
  should pass work over channels or the queue and stop on context
  cancellation; keep mutexes for small caches.
- **Clear is better than clever (13, 14).** Prefer explicit wiring in
  `internal/boot` and plain code in services over reflection-driven magic or
  dense generic helpers.
- **A little copying over a little dependency (8).** Before adding a module to
  `go.mod`, check whether a few lines in-house would do.
- **Make the zero value useful (5).** Config structs and option types should
  behave sensibly when a field is unset; when they cannot, validate at load
  time and fail with a clear error instead of panicking later (19).
