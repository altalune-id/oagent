# @oagent/runtime

The agent runtime: one HTTP service that runs one agent turn per request with Strands Agents and streams the turn back as NDJSON. oagent calls it; it never calls oagent and stores nothing.

## Endpoints

| Route        | Auth                                    | Answer                                                                                                                                                                                                                                                 |
| ------------ | --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET /ping`  | none                                    | `200 ok`                                                                                                                                                                                                                                               |
| `POST /turn` | `Authorization: Bearer <RUNTIME_TOKEN>` | `200 application/x-ndjson`: AG-UI events plus the private `OAGENT_HEARTBEAT`, `OAGENT_CHECKPOINT` and `OAGENT_TERMINAL` (always last). Before the stream: `400` (field paths), `401`, `413` (8 MiB), `422` (snapshot cannot load), `503` (at capacity) |

The request and stream contract is shared with Go through the goldens in `internal/kernel/runtime/testdata/turn/`.

## Environment

| Variable                 | Default         | Meaning                                                                                                                                   |
| ------------------------ | --------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `RUNTIME_TOKEN`          | none (required) | shared bearer, at least 32 bytes; start fails without it                                                                                  |
| `PORT`                   | `5160`          | listen port                                                                                                                               |
| `RUNTIME_MAX_CONCURRENT` | `8`             | turns per instance; over it answers `503`. Set it to at least the sum of the turn caps of the oagent instances that call this one         |
| `RUNTIME_ENV`            | `production`    | `development` or `production`                                                                                                             |
| `RUNTIME_ALLOW_PRIVATE`  | empty           | comma list of exact `host:port` pairs the egress guard may dial even though they are private. Honored only when `RUNTIME_ENV=development` |

`HTTP_PROXY`, `HTTPS_PROXY` and `NODE_USE_ENV_PROXY` are ignored: every outbound call goes through the runtime's own guarded dispatcher, which refuses private, loopback and link-local addresses on every hop.

## Develop

```sh
pnpm --filter @oagent/runtime typecheck
pnpm --filter @oagent/runtime lint
pnpm --filter @oagent/runtime test
```

Run it next to a local `oagent`:

1. Make a token of at least 32 bytes: `openssl rand -hex 32`.
2. Build and start the runtime in development mode, allowing the private hosts it must reach: the local model server and any local MCP server, as exact `host:port` pairs.

   ```sh
   pnpm --filter @oagent/runtime build
   PORT=5170 RUNTIME_TOKEN=<token> RUNTIME_ENV=development RUNTIME_ALLOW_PRIVATE=127.0.0.1:11434,127.0.0.1:5190 \
     node apps/runtime/dist/server.js
   ```

3. Point `oagent` at it: `OAGENT_RUNTIME_URL=http://127.0.0.1:5170` and `OAGENT_RUNTIME_TOKEN=<token>` (config keys `runtime.url`, `runtime.token`).
4. Check it: `curl http://127.0.0.1:5170/ping` answers `ok`.

Set `PORT` to any free port: the default 5160 may already be taken by another local service. Without `RUNTIME_ENV=development` the allow-list is ignored, and the runtime refuses every private address, `localhost` included. A model or MCP server on a public https URL needs no allow-list entry.

## Image

`apps/runtime/Dockerfile`, built from the repository root: `docker build -f apps/runtime/Dockerfile .`. It runs `node dist/server.js` as a non-root user on port 5160. Releases publish `ghcr.io/<owner>/oagent-runtime`, and `main` pushes the `edge` tag.

## Deploy

- Private network only. The runtime has no public domain and no public port. Only the oagent service reaches it, at its private address, with the shared bearer.
- Set `RUNTIME_TOKEN` on both services (oagent's `runtime.token`), and point oagent's `runtime.url` at the runtime's private address.
- Leave `RUNTIME_ENV` and `RUNTIME_ALLOW_PRIVATE` unset in production.
