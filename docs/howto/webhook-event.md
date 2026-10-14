# Add a webhook event

Use this when a tenant's endpoints should hear that something happened in a domain. A webhook payload is a public contract: receivers parse it, so it only ever grows. Work that only oagent itself needs done later is a [worker](worker.md) or a queue job, not an event.

How it flows: the domain service saves its change and calls its event port **in the same unit of work**. `webhook.Service.Enqueue` finds the active endpoints subscribed to the type, builds one signed envelope and writes one `outbox_entries` row per endpoint in that transaction. The `outbox.dispatch` worker later claims due rows per org and `webhook.Deliverer` POSTs them.

## Plan

- [ ] **Name.** `<domain>.<past-tense verb>`: `agent.created`, `invite.created`, `member.added`.
- [ ] **Project or org event.** A project event goes to the endpoints of one project. An org event (`OrgWide: true`) goes to the subscribed endpoints of every project in the org, and only a caller who reaches the whole org may subscribe to it or see its deliveries.
- [ ] **Payload.** Only what a receiver needs. Every id is a **public id** ([public-ids.md](public-ids.md)); never a UUID, a token, a link with a token, a hash or a secret.
- [ ] **Trigger.** Which service method emits it, and whether a failed enqueue should fail that method (it does today: the write rolls back).

## Build

1. **Catalog** in `internal/kernel/events/catalog.go`:
   - a `Type` const (`InviteCreated Type = "invite.created"`);
   - a row in `All()`: `{Type: InviteCreated, Version: 1, Subscribable: true, OrgWide: true}`;
   - a case in `CheckPayload` that accepts only the payload type.
2. **Payload** in `internal/kernel/events/<domain>_payloads.go`: one named `<Event>V1` type per event, even when two share a shape (`AgentCreatedV1 AgentSnapshotV1`). JSON `snake_case`, `time.Time` for times, `omitempty` only for an optional reference. The doc comment says the ids are public ids.

   ```go
   // InviteCreatedV1 is the payload for InviteCreated; ids are public ids and the token or link is never included.
   type InviteCreatedV1 struct {
   	ID        string    `json:"id"`
   	Email     string    `json:"email"`
   	InvitedBy string    `json:"invited_by,omitempty"`
   	ExpiresAt time.Time `json:"expires_at"`
   }
   ```

3. **Catalog tests** in `internal/kernel/events/catalog_test.go`: the new type in `TestCatalog`, a matching and a mismatching payload in `TestCheckPayload`.
4. **Port** in the domain's `ports.go`. The domain imports `internal/kernel/events`, never `internal/webhook`:

   ```go
   // Publisher fans a domain event out to its subscribers; the webhook module implements it.
   type Publisher interface {
   	Enqueue(ctx context.Context, t events.Type, data any) error
   }
   ```

   An org event uses `EnqueueOrg(ctx context.Context, orgID uuid.UUID, t events.Type, data any) error` instead (`invite.Events`). Reuse the domain's port if it already has one.

5. **Emit inside the unit of work.** Save and enqueue in one `RunInTx` closure, so the outbox rows commit with the write or not at all. `Enqueue` reads the org and project from ctx, so bind them first:

   ```go
   // NOTE: one unit of work, so the save and its event commit together or not at all.
   func (s *Service) saveAndPublish(ctx context.Context, t events.Type, a *Agent) error {
   	return s.uow.RunInTx(ctx, func(ctx context.Context) error {
   		if err := s.store.Save(ctx, a); err != nil {
   			return err
   		}
   		return s.publish(ctx, t, a) // tenant.Into(ctx, ...) then s.publisher.Enqueue
   	})
   }
   ```

   A nil port skips emission. An enqueue error is wrapped (`fmt.Errorf("agent: publish %s: %w", t, err)`), rolls the write back and reaches the caller through the service's `unexpected` reporter.

6. **Boot** passes `*webhook.Service` as the port in `buildServices` (`internal/boot/services.go`): `.WithPublisher(webhooks)` or `Deps{Events: webhooks}`.
7. **Console picker:** add `{ name, labelKey }` to `webhookEvents` in `ui/console/app/components/webhook/events.ts` (it is kept in sync with the Go catalog by hand), update `events.test.ts`, and add `webhooks.event_<type with . as _>` to `ui/console/i18n/locales/en-US.json` and `id-ID.json`.
8. **Tests:**
   - service unit tests on a fake publisher: the event fires with the right type and payload (public ids), a publish error fails the call as an internal error, a nil publisher still saves (`internal/agent/publish_test.go`);
   - integration on Postgres with the real store and `db.NewTransactor`: a failing publisher leaves nothing saved, and the publisher runs inside the save transaction (`internal/agent/service_integration_test.go`);
   - an org event: a boot integration test that every project's subscribed endpoint gets a delivery (`internal/boot/invite_events_integration_test.go`).

## Delivery facts you rely on

- Envelope: `id` (event public id), `type`, `api_version` (`v1`), `created_at`, `tenant` (`org_id`, `project_id` public ids, `project_slug`), `data`.
- Headers: `X-Oagent-Event-Id`, `X-Oagent-Event-Type`, `X-Oagent-Delivery-Id`, `X-Oagent-Timestamp`, `X-Oagent-Signature` (`v1=` HMAC-SHA256 over `timestamp + "." + body`; several during a secret rotation). `examples/webhook-pong` verifies them.
- At least once: a repeat of the same event and target is ignored at enqueue, a claim leases for `outbox.ClaimLease` (5 min), and `outbox.MaxAttempts` (8) moves a row to failed, which the console, `oagent webhook retry-failed` and the `webhook_retry_failed` MCP tool can requeue.

## Review

- [ ] Type, `All()` row and `CheckPayload` case agree; `OrgWide` set for an org event.
- [ ] Payload holds public ids only and no token, link, hash or secret.
- [ ] Save and enqueue share one `RunInTx`; ctx carries the right org and project.
- [ ] The domain imports `internal/kernel/events` only; boot wires `*webhook.Service`.
- [ ] Console list, its test and both locales updated.
- [ ] The catalog is additive-only: a shipped type is never removed or renamed, and its payload only gains fields.
- [ ] Gates: `make check`, `make cover` (≥ 85% per package), `make test-integration`, `make ui-test`.

## Reference code

- [internal/kernel/events/catalog.go](../../internal/kernel/events/catalog.go): `Type`, `Spec`, `All`, `CheckPayload`, `IsOrgWide`
- [internal/kernel/events/agent_payloads.go](../../internal/kernel/events/agent_payloads.go), [invite_payloads.go](../../internal/kernel/events/invite_payloads.go)
- [internal/agent/service.go](../../internal/agent/service.go): `saveAndPublish`, `publish`; [internal/agent/ports.go](../../internal/agent/ports.go): `Publisher`
- [internal/org/invite/ports.go](../../internal/org/invite/ports.go): `Events`
- [internal/webhook/service.go](../../internal/webhook/service.go): `Enqueue`, `EnqueueOrg`, `fanOut`, `Envelope`
- [internal/webhook/sign.go](../../internal/webhook/sign.go): `Sign`; [internal/webhook/deliverer.go](../../internal/webhook/deliverer.go): `deliveryHeaders`
- [internal/kernel/outbox/outbox.go](../../internal/kernel/outbox/outbox.go): `MaxAttempts`, `ClaimLease`, `Backoff`
- [internal/boot/outbox.go](../../internal/boot/outbox.go): `buildOutboxWorker`, `outboxTenants`
- [examples/webhook-pong/main.go](../../examples/webhook-pong/main.go)
