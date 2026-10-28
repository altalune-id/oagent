# oagent

oagent is a self-hosted platform for AI agents. It is one Go binary that serves:

- an API (Connect RPC)
- an MCP server
- a CLI
- a web console

It already covers orgs, projects, members, API keys and webhooks. The agent runtime and most product features are still to come.

## Requirements

- Go 1.26.10
- Node 24.15 or newer
- pnpm 10 (`corepack enable` picks the version pinned in `package.json`)
- Docker, for local dependencies and the integration tests

## Set up the repo

1. Clone the repo and install the JavaScript dependencies. This also installs the git hooks.

   ```sh
   git clone <repo-url> oagent && cd oagent
   corepack enable
   pnpm install
   ```

2. Turn on signed commits and the commit template. Pushes with unsigned commits are refused (see [.husky/README.md](.husky/README.md)).

   ```sh
   git config commit.gpgsign true
   git config commit.template .gitmessage.txt
   ```

3. Start Postgres, Redis, NATS and Mailpit.

   ```sh
   docker compose up -d
   ```

4. Create your config. The example DSNs log in as the roles the compose Postgres creates on its first start (`oagent_service` for the app, `oagent_migrator` as `oagent_owner` for migrations); the app refuses to start as a superuser or the schema owner. Generate the two secrets with `openssl rand -hex 32`.

   ```sh
   cp config.example.yml config.yml
   ```

   Every key can also be set from the environment: see `.env.example` and the full list in `config.reference.yml`.

5. Build the embedded UI (console, MCP views, sign-in page assets; the console and sign-in assets also get brotli and gzip copies), then run the server.

   ```sh
   make ui
   make run
   ```

6. Open the one-time install link printed in the log, create the first owner account, and you land in the console at `http://localhost:5150/console/`.

## Provision a production database

`scripts/db/provision.sh` creates the database and three roles on any Postgres (self-hosted, RDS, Neon and others).
It is safe to re-run, and a re-run sets the passwords you pass.

| Role              | Purpose                                                                                       |
| ----------------- | --------------------------------------------------------------------------------------------- |
| `oagent_owner`    | owns the schema, tables and lookup functions; `BYPASSRLS`; nobody logs in as it               |
| `oagent_migrator` | runs migrations as the owner                                                                  |
| `oagent_service`  | used by the running app; reads and writes rows only, under row-level security (`NOBYPASSRLS`) |

```sh
ADMIN_URL='postgres://admin:...@db.example.com:5432?sslmode=require' \
MIG_PW=gen SVC_PW=gen scripts/db/provision.sh
```

- Run it without the variables to be asked for each value.
- `gen` creates a password and writes it to `oagent-db-credentials.env` (mode 0600). Passwords are never printed.
- The script prints the matching `postgres.dsn`, `postgres.migrator.dsn`, `postgres.migrator.role`, `postgres.schema` and `postgres.tablePrefix` settings, then checks the roles and privileges with `scripts/db/verify.template.sql` and fails on drift.
- The app checks the same at boot and refuses to start when it can bypass row-level security.

## Run with Docker

You don't need Go or Node to run oagent. The image holds the same binary with the UI embedded.

```sh
docker build -t oagent .
docker run --rm -p 5150:5150 \
  -e OAGENT_POSTGRES_DSN=postgres://... \
  -e OAGENT_POSTGRES_MIGRATOR_DSN=postgres://... \
  -e OAGENT_HTTP_STATE_SECRET=$(openssl rand -hex 32) \
  -e OAGENT_SECRETS_ENCRYPTION_KEY=$(openssl rand -hex 32) \
  oagent
```

- The image runs `serve` by default. Pass any other command after the image name, for example `docker run --rm oagent version`.
- To run the whole stack in containers: `docker compose --profile app up -d --build`.

### Queue (NATS)

The queue is off unless `queue.enabled` is true; `queue.driver` picks what carries it (`nats`, the only driver).
`nats.*` is only the connection, and oagent opens it only when a feature uses it. oagent connects as user `oagent` in
its own account, `OAGENT`, as on the shared Altalune NATS ([altalune-id/nats](https://github.com/altalune-id/nats)).
Every app uses the same stream names, so the account is what keeps apps apart: never share one between apps.

```sh
OAGENT_QUEUE_ENABLED=true
OAGENT_NATS_URL=nats://<nats-service>.railway.internal:4222
OAGENT_NATS_USER=oagent
OAGENT_NATS_PASSWORD=...   # the server's OAGENT_NATS_PASSWORD, at least 16 characters
```

The credentials may also go in the URL (`nats://oagent:<password>@host:4222`). `nats.token` is for a NATS server
without accounts; set it or the user and password, not both. Deploy the server on private networking only, with a
volume at `/data` (see the shared server's README). Locally, `docker compose up -d` runs NATS with the accounts in
`docker/nats/nats-server.conf` (`OAGENT` and `OTHER`, dev passwords of 16 characters or more). To run oagent on
the host against it: `OAGENT_NATS_URL=nats://localhost:4222`, `OAGENT_NATS_USER=oagent` and
`OAGENT_NATS_PASSWORD=oagent-dev-nats-password` (the compose default).

### Sign in through an OIDC provider

Set `oidc.issuer` to let people sign in through one OpenID Connect provider. Cloud mode requires it, and with
`auth.password.enabled: false` it is the only way in.

```sh
OAGENT_HTTP_BASE_URL=https://oagent.example.com
OAGENT_OIDC_ISSUER=https://idp.example.com          # exactly as the provider names itself, trailing "/" included
OAGENT_OIDC_CLIENT_ID=...
OAGENT_OIDC_CLIENT_SECRET=...
OAGENT_OIDC_BUTTON_LABEL="Continue with Acme ID"   # default "Continue with SSO"
```

At the provider, register a web client with:

- redirect URI `<http.baseURL>/oauth/callback` (`https://oagent.example.com/oauth/callback`);
- post-logout redirect URI `<http.baseURL>/login`;
- the authorization code flow with PKCE `S256` (the discovery document must list `S256`);
- client authentication `client_secret_basic`, or none for a public client without `oidc.clientSecret`;
- the `openid` and `email` scopes (`oidc.scopes` must include both; `profile` adds the name).

With passwords off, `/install` asks only for the first owner's email (headless: `auth.genesis.email` alone);
that person becomes the owner by signing in through the provider with that email.

The provider must send `email` and `email_verified` in the ID token. oagent links a person by the provider's
`sub`; a first sign-in links the existing account with the same verified email, once. oagent trusts that
email, so configure only a provider you trust to verify it. Every link stores `oidc.issuer` (without a trailing
`/`): changing the issuer locks out every linked person, whose next sign-in is refused with `AUT007`.

## Everyday commands

| Command                      | What it does                                                         |
| ---------------------------- | -------------------------------------------------------------------- |
| `make check`                 | vet, every linter, unit tests                                        |
| `make cover`                 | unit + integration tests; fails when a package is under 85% coverage |
| `make ui-test`               | UI tests                                                             |
| `make generate`              | regenerate protos, MCP tools, scopes, error codes and templ views    |
| `make fmt` / `pnpm lint:fix` | format Go / fix JS, TS, Vue, JSON, Markdown and CSS                  |
| `make help`                  | every target                                                         |

## Docs

- [AGENTS.md](AGENTS.md): the rules for contributors and coding agents
- [docs/](docs/README.md): architecture and how-to recipes

## License

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE).
