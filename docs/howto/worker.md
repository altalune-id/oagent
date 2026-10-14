# Add a background worker or job

Use this when work must run outside a request: a polling loop, a periodic job, or a queue handler. Everything long-running is a `worker.Worker` registered on the one `Supervisor` that `Boot` builds; `App.Run` runs them all under one errgroup until SIGINT or SIGTERM cancels the context (`cmd/oagent/main.go`).

Pick the shape first:

| I want                                                                   | Shape                                                                                       |
| ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| a loop on every replica (drain a table, batch writes, ping a dependency) | a `worker.Worker` (`outbox.Worker`, `mailqueue.Worker`, `apikey.UsageWorker`)               |
| a periodic job on an interval, daily or cron schedule                    | a `scheduler.Job` from a domain's `SchedulerJobs()` (`session.Scheduler`)                   |
| work done once, retried and dead-lettered, on the queue                  | a `queue.Handler` (none exist yet)                                                          |
| to tell a tenant something happened                                      | none of these: a [webhook event](webhook-event.md), delivered by the existing outbox worker |

## Plan

- [ ] **Where it lives.** Next to the data it owns: a domain (`internal/apikey/usage.go`), a kernel package (`internal/kernel/outbox/worker.go`), or a kernel feature's driver (`internal/kernel/ratelimit/memory`).
- [ ] **When it runs.** Always, or only when a feature is on (the janitor only with the memory rate limit store, `redis-health` only when Redis is used).
- [ ] **Tenancy.** A loop over tenant rows takes a `Tenants` port and binds each org with `tenant.Into` (`outboxTenants` in `internal/boot/outbox.go`). System work binds nothing.
- [ ] **Shutdown.** What must finish when the context is cancelled, and in how long. The HTTP server gets 10 s; keep a final drain within a few seconds.
- [ ] **Tunables** are package constants or an `Opts` struct with defaults, not config keys, unless an operator truly must set them ([config-key.md](config-key.md)).

## Build: a worker.Worker

1. **The type** with `Name()` and `Run(ctx)`, plus `var _ worker.Worker = (*Worker)(nil)`. One constructor; `*slog.Logger` and ports are injected; no goroutine starts in the constructor.

   ```go
   // Run implements worker.Worker, sweeping due entries every tick until ctx is done.
   func (w *Worker) Run(ctx context.Context) error {
   	t := time.NewTicker(w.opts.Tick)
   	defer t.Stop()
   	for {
   		select {
   		case <-ctx.Done():
   			return nil
   		case <-t.C:
   			w.sweep(ctx)
   		}
   	}
   }
   ```

   - A ticker and `ctx.Done()`, never `time.Sleep` in a loop.
   - A failed tick is logged (or reported) and the loop goes on. Returning an error from `Run` stops every worker and the process.
   - On cancel return `nil` or `ctx.Err()`: `Supervisor.Run` treats `context.Canceled` during shutdown as a clean exit.
   - Work that must survive the cancel (a final flush) uses `context.WithTimeout(context.WithoutCancel(ctx), …)`, as `UsageWorker.Run` does.
   - A worker whose dependencies are nil logs once and returns `nil` (`outbox.Worker.Run`).

2. **Name** in the existing style: `<package>.<verb>` for kernel workers (`outbox.dispatch`, `mailqueue.send`), a short kebab name otherwise (`apikey-usage`, `db-health`). Export it as `WorkerName` when tests or boot refer to it.
3. **Boot.** Build it in the file that owns the feature (`internal/boot/outbox.go`, `mail.go`, `ratelimit.go`) and `sup.Register(...)` it in `Boot` (`internal/boot/boot.go`), behind its condition. A resource it holds is closed through `k.AddCloser`; the kernel closes in reverse order.
4. **Tests:** unit tests on fakes for one tick, a failing tick that keeps running, and `Run` returning when the context is cancelled (`TestWorkerRunTicksThenStops`). Update the worker counts in `internal/boot/boot_test.go`.

## Build: a scheduler job

1. In the owning package, a `Scheduler` adapter that implements `scheduler.Provider`:

   ```go
   func (a *Scheduler) SchedulerJobs() []scheduler.Job {
   	return []scheduler.Job{{
   		Name: sweepJobName, Scope: scheduler.ScopeSystem,
   		Schedule: scheduler.MustEveryInterval(sweepEvery, sweepJitter),
   		Timeout: sweepTimeout, Singleton: true,
   		Run: func(ctx context.Context) error { /* one call into the store or service */ },
   	}}
   }
   ```

   Cadence, jitter and timeout are package constants. `Run` returns its error; the runner reports it.

2. Register it in `buildScheduler` (`internal/boot/schedulers.go`). The runner itself is one worker, on when `scheduler.enabled` and not turned off by `serve --no-scheduler` or consumer-only; `serve --scheduler-only` runs it alone.
3. `ScopeTenant` needs `scheduler.Options.Tenants` and `Singleton` needs `Options.Locker`; `schedulerConfig` sets neither today, so the first job that needs one wires it there.

## Build: a queue handler

`queue.Handler{Job, Handle, OnDeadLetter}` in the owning package; decode with `queue.Decode[T]`, return `queue.Permanent(err)` for a bad payload, call one service method, keep it idempotent (`m.ID` dedupes). Return it from `queueHandlers` (`internal/boot/consumers.go`). Code that submits takes `queue.Queue` (`Kernel.Queue`), never a driver: only the `nats` connection, `internal/kernel/queue/nats`, boot and `internal/testutil/natstest` import NATS (depguard `nats-boundary`). `Queue.Submit` refuses a job that was not declared, and nothing calls `Queue.Declare` yet, so the first job also adds that call in boot. The consumer runs only with `queue.enabled`; with it off, `queue.Disabled` accepts every Submit and does nothing.

## Review

- [ ] `Run` never returns an error for a single failed tick.
- [ ] No `time.Sleep` loop; the ticker is stopped; cancel is honoured within the shutdown window.
- [ ] Tenant data is read under a bound org scope, never across tenants.
- [ ] Registered in `Boot` behind the right condition; closers added for held resources.
- [ ] `internal/boot/README.md` updated in the same commit: the boot order diagram if a step changed, the **Background workers** table (name, what, when) and the **Files** table.
- [ ] Gates: `make check`, `make cover` (≥ 85% per package), `make test-integration` when it touches Postgres or Redis.

## Reference code

- [worker/worker.go](../../worker/worker.go), [worker/supervisor.go](../../worker/supervisor.go): `Worker`, `Supervisor.Run`
- [internal/kernel/outbox/worker.go](../../internal/kernel/outbox/worker.go): `Worker`, `WorkerOpts`, `Tenants`
- [internal/kernel/mailqueue/worker.go](../../internal/kernel/mailqueue/worker.go): `Worker`
- [internal/apikey/usage.go](../../internal/apikey/usage.go): `UsageWorker` (drain on shutdown)
- [internal/kernel/ratelimit/memory/memory.go](../../internal/kernel/ratelimit/memory/memory.go): `Store.Run` (a janitor on a store)
- [internal/kernel/session/scheduler.go](../../internal/kernel/session/scheduler.go): `Scheduler.SchedulerJobs`
- [scheduler/scheduler.go](../../scheduler/scheduler.go), [scheduler/provider.go](../../scheduler/provider.go): `Job`, `Provider`, `Locker`
- [internal/kernel/queue/queue.go](../../internal/kernel/queue/queue.go): `Queue`; [internal/kernel/queue/handler.go](../../internal/kernel/queue/handler.go): `Handler`, `Decode`, `Permanent`
- [internal/kernel/queue/nats/consumer.go](../../internal/kernel/queue/nats/consumer.go): the NATS driver's consumer (retries, dead letters)
- [internal/boot/boot.go](../../internal/boot/boot.go): `Boot`; [internal/boot/schedulers.go](../../internal/boot/schedulers.go): `buildScheduler`; [internal/boot/consumers.go](../../internal/boot/consumers.go): `queueHandlers`
- [internal/boot/README.md](../../internal/boot/README.md): Background workers
