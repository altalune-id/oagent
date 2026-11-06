# Data plane API (`/api/v1/`)

The data plane is how an integrator's backend talks to an agent. You send one user message and read the agent's turn back as an [AG-UI](https://docs.ag-ui.com) event stream. Any AG-UI HTTP client (`@ag-ui/client` `HttpAgent`) works unchanged.

Every path is under `{http.basePath}/api/v1/agents/{agent}`. `{agent}` is the agent's public id.

## Authentication

- **API keys only:** `Authorization: Bearer <key>`, with a project, org or personal key. There are no cookies and no CORS, so call it from your server, never from a browser.
- **Hidden agents:** an agent the key does not reach answers `404 AGT001`, the same as an agent that does not exist.
- **Scopes:** `POST …/runs` and `POST …/cancel` need `run:create`. The `GET`s need `run:read`. A missing scope answers `403 GEN007` with `meta.permission`.

## End users

A conversation belongs to one end user, called the actor.

- **Project and org keys:** name the actor with the `Oagent-User: <your user id>` header (`^[A-Za-z0-9._:@-]{1,128}$`).
- **Reserved prefixes:** `Oagent-User` values starting with `key:` or `person:` (any case) are refused with `400`; those prefixes name oagent's own actors.
- **Default actor:** without the header, the actor is the key itself.
- **Personal keys:** the actor is always the key's owner, and sending `Oagent-User` answers `403`.
- **Binding:** a thread is bound to its first actor. Another actor asking for the same `threadId` gets `404 THR001`, exactly as if the thread did not exist.

## Endpoints

### `POST /runs`: run one turn

The body is AG-UI `RunAgentInput`:

```json
{ "threadId": "conv-42", "messages": [{ "id": "m1", "role": "user", "content": "hello" }] }
```

- **`threadId`:** your conversation key (`^[A-Za-z0-9._:@-]{1,128}$`). The first POST creates the thread.
- **`messages`:** only the **last** message is used, and it must be `role: user`. Its content is a string or text parts, at most 32 KiB. oagent keeps the history, so the earlier messages are ignored.
- **Other fields:** `runId`, `parentRunId`, `state`, `tools`, `context`, `forwardedProps`, `protocolVersion` and `resume` are accepted and ignored.
- **Strictness:** any other top-level key answers `400`. The body is at most 64 KiB (`413`).
- **`Idempotency-Key: <key>` (optional):**
  - Retrying with the same key and body never starts a second turn.
  - If that turn is still running, the retry answers `409 RUN007` with `meta.run`.
  - If it has ended, the retry replays it as a short stream built from the stored messages.
  - The same key with another body answers `422 RUN008`.

The response is `200 text/event-stream`:

```text
id: 1
data: {"type":"RUN_STARTED","threadId":"conv-42","runId":"Rn5Q8wYk2HhT3mVz7XcLp"}

id: 2
data: {"type":"TEXT_MESSAGE_START","messageId":"…","role":"assistant"}

: ping

id: 5
data: {"type":"RUN_FINISHED","threadId":"conv-42","runId":"Rn5Q8wYk2HhT3mVz7XcLp"}
```

- **Events:** `RUN_STARTED`, `TEXT_MESSAGE_START`/`CONTENT`/`END`, `TOOL_CALL_START`/`ARGS`/`END`, `TOOL_CALL_RESULT`, then `RUN_FINISHED` or `RUN_ERROR`.
- **Ids:** `threadId` is always your own key, and `runId` is the run's public id.
- **Heartbeat:** `: ping` comments keep the connection open, at least every 15 s.
- **The last event is sent only once the run is stored.** `RUN_ERROR` carries `{message, code}` and no `runId`. Its `code` is:
  - `cancelled`: the run was cancelled.
  - `interrupted`: the runtime went away mid-turn. The run resumes on the next turn of the thread.
  - a run error code, such as `RUN004`.
  - `detached`: the stream ended before the run did. `GET …/runs/{run}` shows how it ended.
- **Disconnects:** dropping the connection does **not** stop the turn. Reload the thread with `GET …/messages`. The stream exists only on the oagent instance that took the POST, and reconnecting does not re-attach to it.

Errors before the stream starts:

| Status | Code     | When                                                                                             |
| ------ | -------- | ------------------------------------------------------------------------------------------------ |
| 409    | `AGT006` | the agent has no published version                                                               |
| 409    | `RUN007` | a turn is already running on this thread                                                         |
| 429    | `RUN002` | the project is at its run rate or concurrency cap, or a data-plane rate limit; see `Retry-After` |
| 503    | `RUN003` | no agent runtime is configured, or every turn slot is busy; see `Retry-After`                    |

None of these refusals stores your message, with one exception: a `503 RUN003` that comes after the message was stored means it will still run. Retry with the same `Idempotency-Key` to get its stream or its result, never a second turn.

### `GET /runs/{run}`

Returns `{id, thread_id, status, attempt, usage:{input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, estimated}, cost_micros, error_code, started_at, ended_at}`. `cost_micros` is `null` when the run is unpriced.

### `POST /runs/{run}/cancel`

Answers `202` with the run.

- **A live run** stops at its next step, and its stream ends with `RUN_ERROR` `cancelled`.
- **A run that already ended** is returned as it is, so cancel is safe to retry.

### `GET /threads/{thread}/messages?after=<seq>&limit=<n>`

Lists messages oldest first, after `after` (default 0), `limit` at a time (default 50, at most 200).

- **Shape:** `{messages:[{seq, status, message}], next_after}`. `message` is the AG-UI message.
- **Paging:** pass `next_after` back as `after` to page.

## Errors

Every error is JSON: `{"code": "...", "message": "...", "request_id": "...", "meta": {...}}`, with `X-Request-Id` and `Cache-Control: no-store`.

## Limits

- **Project cap:** each POST that starts a turn counts once against the project's `run.maxPerMinutePerProject`.
- **`rateLimit.dataPlane`** (on by default): every call counts per client IP (`perIPPerMinute`), then per API key (`perKeyPerMinute`). Behind a proxy, set `http.trustedProxyHops`.

## Example

```sh
curl -N "$OAGENT/api/v1/agents/$AGENT/runs" \
  -H "Authorization: Bearer $OAGENT_KEY" -H "Oagent-User: alice" -H "Content-Type: application/json" \
  -d '{"threadId":"conv-42","messages":[{"id":"m1","role":"user","content":"hello"}]}'
```
