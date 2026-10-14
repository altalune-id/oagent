# Rate-limit an action

Use this when an action needs oagent's own limiter: a form that can be brute-forced or used to spam mail. oagent's limiter is **off by default**, because many deployments limit at the edge (a CDN, nginx, an API gateway). Each **group** of limits has its own switch, and every group shares one store. The only group today is `auth` (sign-in, password reset, invite pages); it is applied on pre-sign-in web forms only.

The `internal/kernel/ratelimit` feature is generic: a `Rule` (GCRA: `Rate` calls per `Per`, `Burst` at once), a key, and a `Limiter` with `Allow` and `Reset`. Drivers, picked by `rateLimit.driver`: `ratelimit/memory` (per server, bounded, with a janitor worker) and `ratelimit/redis` (one Lua script over the shared Redis connection, so every server counts against one limit).

## Plan

- [ ] **Does it belong in the `auth` group?** A pre-sign-in form does. Something else (an API or MCP call counted per key or per org) is a new group with its own switch, `rateLimit.<group>.enabled`. No such group exists yet.
- [ ] **Subject.** Per client IP (`ratelimit.IPSubject(layout.ClientIP(r, hops))`), per email (`ratelimit.EmailSubject`, a SHA-256, so no key holds an address), or both. Count the IP first.
- [ ] **Numbers.** `Rate`, `Per`, `Burst`, written in the rule's doc comment ("60 an hour per IP").
- [ ] **Failures only or every call.** Login counts every attempt per IP and clears the email key on success (`ForgetEmail`), so only failures stay counted per email.
- [ ] **No enumeration.** A known and an unknown email get the same status, body and headers, with the limiter on or off.
- [ ] **Store failure.** The `auth` group fails closed: a limiter error answers 503, never lets the call through.

## Build: a limit in the auth group

1. **Rule** as an exported func next to the surface that uses it (a func, not a var: `gochecknoglobals`). The name is kebab-case `<action>-<subject>` and unique across oagent, because it namespaces the store key (`<redis.keyPrefix>rl:<rule>:<subject>`):

   ```go
   // SubmitRule limits joining and accepting per client IP: 20 an hour.
   func SubmitRule() ratelimit.Rule {
   	return ratelimit.Rule{Name: "invite-submit-ip", Rate: 20, Per: time.Hour, Burst: 20}
   }
   ```

   Auth rules live in `internal/auth/limits.go`; invite rules in `internal/org/invite/web.go`.

2. **Apply it in the web handler** through `auth.Limits`, which the page's options already carry. Never call the `Limiter` directly: `Limits` builds the key and maps a refusal to `*auth.RateLimitedError`.

   ```go
   func (h *Web) limited(rule ratelimit.Rule, next http.HandlerFunc) http.HandlerFunc {
   	return func(w http.ResponseWriter, r *http.Request) {
   		if err := h.o.Limits.LimitIP(r, rule); err != nil {
   			auth.WriteLimited(w, r, h.o.Kit, "Invite", err)
   			return
   		}
   		next(w, r)
   	}
   }
   ```

   `auth.WriteLimited` answers 429 with `Retry-After` and a generic message, or 503 and reports the error once. A JSON caller of `/login` gets `AUT003` with `retry_after` (`RateLimitedError.ToAppError`).

3. **A new page** gets `auth.Limits` in its options from `authLimits(cfg, svcs)` in `buildRoutes` (`internal/boot/http.go`). It carries the auth group's limiter and `http.trustedProxyHops`.
4. **Tests** in the page's `web_limits_test.go`, with the memory store (`memory.New(10_000)`) for "on" and `ratelimit.Disabled()` (or the zero `auth.Limits`) for "off": limited after `Burst` calls with 429 and `Retry-After`; per email across IPs and per IP across emails where both apply; a broken limiter gives 503 (fail closed); off by default never limits; known and unknown emails get identical answers.

## Build: a new group

1. **Config** (`config/ratelimit.go`): a `RateLimitGroupConfig` field on `RateLimitConfig`, added to `AnyEnabled`; `v.SetDefault("rateLimit.<group>.enabled", false)` in `setDefaults`. Follow [config-key.md](config-key.md) and run `make config-examples`.
2. **Boot** (`internal/boot/ratelimit.go`): a field on `limiters`, defaulting to `ratelimit.Disabled()` and set to the shared store when the group is on; a field on `Services` (like `AuthLimiter`) set in `Boot`. Tests next to `TestBuildLimiters_AuthOnWithMemory`.
3. **A limits helper** for that surface, in the shape of `auth.Limits` (a zero value that limits nothing).

## Build: a new store

A driver subpackage `ratelimit/<driver>` that implements `ratelimit.Limiter`, validates the rule, and returns an error (never an allowed call) when its backend fails. It runs the shared contract, `ratelimittest.Run(t, func(t *testing.T, clock *ratelimittest.Clock) ratelimit.Limiter {...})`. Add it to the `oneof` tag on `rateLimit.driver` and the switch in `buildLimiters`. A store on Redis takes the plain `redis.UniversalClient` and a prefix ending in its namespace, and the feature that selects it is listed in `Config.RedisFeatures`, so Redis is opened only then.

## Review

- [ ] Rule is a func with a unique kebab-case name and its numbers in the doc comment.
- [ ] Applied through `auth.Limits` and answered through `auth.WriteLimited`; no direct `Limiter` call in a handler.
- [ ] No key, log line or message holds a raw email or password; email subjects are hashed.
- [ ] Same answer for known and unknown accounts, limiter on and off.
- [ ] Off by default; a group that is off gets `ratelimit.Disabled()` and starts no janitor.
- [ ] `internal/boot/README.md` **Rate limits** section updated in the same commit (where each group applies, its numbers, its keys).
- [ ] Gates: `make check`, `make cover` (≥ 85% per package), `make test-integration` (Redis store and boot), `make config-check` for a new key.

## Reference code

- [internal/kernel/ratelimit/ratelimit.go](../../internal/kernel/ratelimit/ratelimit.go): `Limiter`, `Rule`, `Result`; [internal/kernel/ratelimit/key.go](../../internal/kernel/ratelimit/key.go): `Key`, `IPSubject`, `EmailSubject`; [internal/kernel/ratelimit/disabled.go](../../internal/kernel/ratelimit/disabled.go): `Disabled`
- [internal/kernel/ratelimit/memory/memory.go](../../internal/kernel/ratelimit/memory/memory.go): `Store`, `Store.Run` (janitor)
- [internal/kernel/ratelimit/redis/redis.go](../../internal/kernel/ratelimit/redis/redis.go), [internal/kernel/ratelimit/redis/gcra.lua](../../internal/kernel/ratelimit/redis/gcra.lua)
- [internal/kernel/ratelimit/ratelimittest/contract.go](../../internal/kernel/ratelimit/ratelimittest/contract.go): `Run`, `Clock`
- [internal/auth/limits.go](../../internal/auth/limits.go): `Limits`, `WriteLimited`, `LoginIPRule`, `LoginEmailRule`
- [internal/auth/web.go](../../internal/auth/web.go): `limitLogin`; [internal/org/invite/web.go](../../internal/org/invite/web.go): `limited`, `LandingRule`, `SubmitRule`
- [internal/web/layout/clientip.go](../../internal/web/layout/clientip.go): `ClientIP`
- [config/ratelimit.go](../../config/ratelimit.go): `RateLimitConfig`, `AnyEnabled`; [config/redis.go](../../config/redis.go): `RedisFeatures`
- [internal/boot/ratelimit.go](../../internal/boot/ratelimit.go): `buildLimiters`; [internal/boot/http.go](../../internal/boot/http.go): `authLimits`
- [internal/auth/web_limits_test.go](../../internal/auth/web_limits_test.go)
- [internal/boot/README.md](../../internal/boot/README.md): Redis, Rate limits
