# @oagent/runtime

The agent runtime: one HTTP service that runs one agent turn per request with Strands Agents and streams the turn back as NDJSON. oagent calls it; it never calls oagent and stores nothing.

## Endpoints

| Route        | Auth                                    | Answer                                                                                                                                                                                                                                                                                                                                 |
| ------------ | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /ping`  | none                                    | `200 ok`                                                                                                                                                                                                                                                                                                                               |
| `POST /turn` | `Authorization: Bearer <RUNTIME_TOKEN>` | `200 application/x-ndjson`: AG-UI events plus the private `OAGENT_HEARTBEAT`, `OAGENT_CHECKPOINT` and `OAGENT_TERMINAL` (always last). Before the stream: `400` (field paths), `401`, `413` (24 MiB), `422` (snapshot cannot load, or image input refused: `image_invalid`, `image_too_large`, `too_many_images`), `503` (at capacity) |

The request and stream contract is shared with Go through the goldens in `internal/kernel/runtime/testdata/turn/`.

## Environment

`src/config.ts` parses every variable once at start, with zod. A bad value stops the start and names the field, never its value. An empty value counts as unset.

| Variable                    | Default         | Meaning                                                                                                                                   |
| --------------------------- | --------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `RUNTIME_TOKEN`             | none (required) | shared bearer, at least 32 bytes; start fails without it                                                                                  |
| `PORT`                      | `5160`          | listen port                                                                                                                               |
| `RUNTIME_MAX_CONCURRENT`    | `8`             | turns per instance; over it answers `503`. Set it to at least the sum of the turn caps of the oagent instances that call this one         |
| `RUNTIME_ENV`               | `production`    | `development` or `production`                                                                                                             |
| `RUNTIME_ALLOW_PRIVATE`     | empty           | comma list of exact `host:port` pairs the egress guard may dial even though they are private. Honored only when `RUNTIME_ENV=development` |
| `RUNTIME_SHUTDOWN_GRACE_MS` | `25000`         | how long a stop waits for running turns before it cuts them                                                                               |
| `RUNTIME_LOG_LEVEL`         | `info`          | `debug`, `info`, `warn` or `error`                                                                                                        |
| `RUNTIME_LOG_FORMAT`        | `text`          | `text` or `json`; set `json` in production                                                                                                |

`HTTP_PROXY`, `HTTPS_PROXY` and `NODE_USE_ENV_PROXY` are ignored: every outbound call goes through the runtime's own guarded dispatcher, which refuses private, loopback and link-local addresses on every hop.

Logs keep only an allow-list of fields (`src/log.ts`), so request secrets never reach them, and an error logs its class name only.

## Shutdown

On `SIGTERM` or `SIGINT` the runtime:

1. Answers `503` to every new request, which oagent retries later.
2. Stops listening and closes idle connections.
3. Waits up to `RUNTIME_SHUTDOWN_GRACE_MS` for running turns to end.
4. Cuts the turns still running. oagent marks the run interrupted and retries it from its stored checkpoints.
5. Exits `0`, or `1` if closing failed, it took 5 s longer than the grace period, or a second signal came.

The platform's stop timeout must be longer than the grace period.

## Layout

| Path              | Owns                                                                  |
| ----------------- | --------------------------------------------------------------------- |
| `src/main.ts`     | composition root: config, logger, egress guard, server, shutdown      |
| `src/config.ts`   | the environment schema                                                |
| `src/log.ts`      | pino logger with the field allow-list                                 |
| `src/shutdown.ts` | signals, ordered close, force exit                                    |
| `src/contract/`   | the turn request schema shared with Go, image checks, refusals        |
| `src/http/`       | routes, bearer check, body cap, NDJSON stream, drain                  |
| `src/turn/`       | one agent turn: execution, AG-UI events, history, snapshot, limits    |
| `src/models/`     | model factory, OpenAI-compatible client, client cache                 |
| `src/mcp/`        | per-turn MCP clients, auth headers, description and schema sanitizing |
| `src/egress/`     | SSRF guard and guarded fetch                                          |

Each folder's `index.ts` is its entry point; other folders import only that file.

## Develop

```sh
pnpm --filter @oagent/runtime typecheck
pnpm --filter @oagent/runtime lint
pnpm --filter @oagent/runtime test
```

Run it next to a local `oagent`:

1. Copy `apps/runtime/.env.example` to `apps/runtime/.env`, and set `RUNTIME_TOKEN` to `openssl rand -hex 32`.
2. List in `RUNTIME_ALLOW_PRIVATE` the private hosts it must reach: the local model server and any local MCP server, as exact `host:port` pairs.
3. Start it: `pnpm --filter @oagent/runtime dev`. It runs the TypeScript sources directly and restarts on change.
4. Point `oagent` at it: `OAGENT_RUNTIME_URL=http://127.0.0.1:5170` and `OAGENT_RUNTIME_TOKEN=<token>` (config keys `runtime.url`, `runtime.token`).
5. Check it: `curl http://127.0.0.1:5170/ping` answers `ok`.

Set `PORT` to any free port: the default 5160 may already be taken by another local service. Without `RUNTIME_ENV=development` the allow-list is ignored, and the runtime refuses every private address, `localhost` included. A model or MCP server on a public https URL needs no allow-list entry.

## Image

`apps/runtime/Dockerfile`, built from the repository root: `docker build -f apps/runtime/Dockerfile .`. It runs `node dist/main.js` as a non-root user on port 5160. Releases publish `ghcr.io/<owner>/oagent-runtime`, and `main` pushes the `edge` tag.

## Deploy

- Private network only. The runtime has no public domain and no public port. Only the oagent service reaches it, at its private address, with the shared bearer.
- Set `RUNTIME_TOKEN` on both services (oagent's `runtime.token`), and point oagent's `runtime.url` at the runtime's private address.
- Leave `RUNTIME_ENV` and `RUNTIME_ALLOW_PRIVATE` unset in production.
