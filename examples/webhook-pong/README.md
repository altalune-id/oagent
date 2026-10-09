# webhook-pong

Localhost webhook receiver for testing: verifies `X-Oagent-Signature`, logs each delivery, and always replies `200 PONG!!!`.

## Run

```bash
make webhook-pong                              # builds ./bin/webhook-pong
set -a; . examples/webhook-pong/.env; set +a   # loads OAGENT_WEBHOOK_SECRET
./bin/webhook-pong
```

Env: `OAGENT_WEBHOOK_SECRET` (signing secret; unset ⇒ signatures not verified, prints a warning), `ADDR` (default `127.0.0.1:9099`).

## Point a webhook at it

Console → **Project → Webhooks → New endpoint**, URL `http://127.0.0.1:9099/`, subscribe to `agent.created` / `agent.updated`, and copy the secret (shown once) into `.env`. Then create/update an agent to trigger a delivery.

> Non-production only: dev allows `http://` + loopback. Production requires `https` and blocks private hosts (SSRF).

## Signature

`X-Oagent-Signature` = `v1=` + `HMAC-SHA256(secret, X-Oagent-Timestamp + "." + rawBody)`, timestamp in unix seconds (±5 min). During rotation both signatures are sent, space-separated.
