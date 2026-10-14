# Add a config key

Use this when an operator needs a new setting. The loader reads defaults, then a YAML file, then `OAGENT_*` env vars. Only `internal/boot`, `internal/cli`, `internal/kernel/capabilities` and the config tools import `config`: a domain never reads a key itself, boot hands it the value.

## Plan

- [ ] **Is it a knob at all?** A cadence, a batch size or a limit that changes domain behaviour stays a constant in the owning package (`outbox.MaxAttempts`, `auth.LoginIPRule`). A key is for something an operator must pick per deployment.
- [ ] **Name and block.** `camelCase` YAML segments under the block that owns it (`rateLimit.maxKeys`, `redis.connectTimeout`). A new block gets its own file (`config/redis.go`) when it brings types or rules.
- [ ] **Default.** Pick a safe one so most deployments set nothing.
- [ ] **Required or optional?** `.env.example` and `config.example.yml` stay minimal: only what selfhosted needs to start (`postgres.dsn`, `postgres.migrator.dsn`, `http.stateSecret`, `secrets.encryptionKey`). Prefer a safe default over a new required key.
- [ ] **Awareness.** `required`, `bootstrap` (locked at first boot), `secret` (never printed), or nothing.
- [ ] **Rule.** One field alone: a `validate:"..."` tag. Depends on another field or the environment: a function.

## Build

1. **Field** on the owning struct in `config/config.go` (or the block's own file). Tag order: `yaml`, `mapstructure`, `awareness`, `validate`. `mapstructure` is what the loader and the env walker key on; `yaml` names the key in errors.

   ```go
   type RedisConfig struct {
   	URL       string `yaml:"url"       mapstructure:"url"       awareness:"secret"`
   	KeyPrefix string `yaml:"keyPrefix" mapstructure:"keyPrefix"`
   }
   ```

   A long tag line takes `//nolint:lll // struct tags are single-line`, as on `Config.Mode`.

2. **Default** in `setDefaults` (`config/loader.go`): `v.SetDefault("rateLimit.maxKeys", 100_000)`. Durations are Go duration strings (`"5s"`).
3. **Env var.** Nothing to register. `bindEnv` (`config/env.go`) walks the struct: prefix `OAGENT`, `.` and `-` become `_`, camelCase splits, all upper case. `rateLimit.maxKeys` is `OAGENT_RATE_LIMIT_MAX_KEYS`. `config.EnvVar(key)` returns the name.
4. **Single-field rule** as a tag: `required`, `oneof=memory redis`, `gte=0`, `omitempty,url`, `datetime=2006-01-02`, or the custom tags `mail_address` and `secret` registered in `newValidator` (`config/validate.go`). A new parse check is a new custom tag there, plus its reason in `tagReason`.
5. **Cross-field rule** as a method `func (c *Config) validate<Block>() error`, appended to the list in `Config.Validate` (`config/config.go`). It returns the shared error types, named by the YAML key, and never the value:

   ```go
   if c.RateLimit.Driver == RateLimitDriverMemory && c.RateLimit.MaxKeys < 1 {
   	return &InvalidSettingError{Key: "rateLimit.maxKeys", Reason: "must be at least 1"}
   }
   ```

   Use `*MissingSettingError{Keys, Reason}` when a choice makes a key required (`redis.url` once a feature selects Redis). Tags run first and are joined with `errors.Join`; functions run only when every tag passes, so they may assume well-formed fields.

6. **Tests** in `config/<block>_test.go`: `TestLoad_<Block>Defaults`, `TestLoad_<Block>FromEnv`, `TestValidate_<Block>`. Add one valid and one invalid row per rule to `TestValidate_EveryRule` (`config/validate_test.go`). A secret key gets a row in `TestValidate_NoSecretInMessages`.
7. **Boot** maps the value onto the consumer's own options (`natsConfig` in `internal/boot/nats.go`, `redisConfig` in `internal/boot/redis.go`, `buildLimiters` in `internal/boot/ratelimit.go`). Root and kernel packages (`scheduler`, `worker`, the connections `pg`, `nats`, `redis`, the kernel features) carry no `mapstructure` tags and never import `config` (depguard `config-boundary` checks the kernel and the connections); one that needs settings has its own `Config` in its `config.go`, and boot maps the app config into it (`writerConfig` in `internal/boot/db.go`, `capabilitiesConfig` in `internal/boot/services.go`).
   A connection (`postgres.*`, `redis.*`, `nats.*`) is its own block with everything backend-specific, separate from the features that use it; each feature picks its driver in its own block (`db.driver`, `rateLimit.driver`, `queue.driver`) and `<Backend>Features()` (`PostgresFeatures`, `RedisFeatures`, `NATSFeatures`) decides whether boot opens the connection at all.
8. **Reference files.** Run `make config-examples`; it rewrites `.env.reference` and `config.reference.yml` through `cmd/gen-config-example`. Never edit those two by hand. Edit `.env.example` and `config.example.yml` by hand, and only for a key that is truly required, with a one-line hint (`# generate: openssl rand -hex 32`).
9. **Docs.** When the key changes wiring, a worker or a feature's behaviour, update `internal/boot/README.md` in the same commit (the Postgres, Redis, NATS and Rate limits sections list their keys).

## Review

- [ ] Tags in order; `mapstructure` matches `yaml`.
- [ ] Default in `setDefaults`, or the zero value is the intended default.
- [ ] A secret has `awareness:"secret"`; no error, log line or `Reason` contains the value.
- [ ] Single-field rules are tags; only cross-field or environment rules are functions.
- [ ] Every error is `*InvalidSettingError` or `*MissingSettingError` with the YAML key.
- [ ] No domain package imports `config`.
- [ ] `.env.example` and `config.example.yml` unchanged unless the key is required to start.
- [ ] `make config-check` clean (CI and the pre-push hook run it).
- [ ] Gates: `make check`, `make cover` (config ≥ 85%).

## Reference code

- [config/config.go](../../config/config.go): `Config`, `Config.Validate`, `HTTPConfig.TrustedProxyHops`
- [config/ratelimit.go](../../config/ratelimit.go): `RateLimitConfig`, `validateRateLimit` (a small block)
- [config/postgres.go](../../config/postgres.go): `DBConfig` (the feature, `db.driver`), `PostgresConfig` (the connection), `PostgresFeatures`, `validatePostgres`
- [config/redis.go](../../config/redis.go): `RedisConfig`, `RedisFeatures`, `validateRedis` (conditional required)
- [config/nats.go](../../config/nats.go): `NATSConfig`, `NATSFeatures`, `validateNATS`; [config/queue.go](../../config/queue.go): `QueueConfig`, `QueueDriver` (a feature that picks its driver)
- [config/validate.go](../../config/validate.go): `newValidator`, `settingError`, `tagReason`
- [config/errors.go](../../config/errors.go): `InvalidSettingError`, `MissingSettingError`
- [config/loader.go](../../config/loader.go): `setDefaults`
- [config/env.go](../../config/env.go), [config/envwalk.go](../../config/envwalk.go): `bindEnv`, `envVarName`, `mergeAwareness`
- [config/validate_test.go](../../config/validate_test.go): `TestValidate_EveryRule`
- [cmd/gen-config-example/main.go](../../cmd/gen-config-example/main.go)

Related: [rate-limit.md](rate-limit.md), [worker.md](worker.md).
