# Connect a WhatsApp device through OpenWA

Use this to let an agent answer WhatsApp messages. OpenWA holds the WhatsApp device and posts each matched message to oagent; oagent runs the agent and sends the reply back through OpenWA. The steps follow the order OpenWA needs: oagent's webhook URL first, then the signing secret OpenWA mints for it.

## Before you start

- The server has a public URL in `http.baseURL`. Without it, creating a connection fails with `CHN006`, because OpenWA would have nowhere to post.
- An OpenWA device, paired, with an API key bound to it that holds `messages:write` and `messages:read`.
- The agent is published, on a model marked **vision** if it should read photos.
- OpenWA on a private network over http works outside production; in production set `channel.allowPrivateHosts: true`.

## Connect

1. In the console, open the agent, then **Channels**, then **OpenWA**. Enter the OpenWA base URL (`channel.defaultOpenwaBaseURL` fills it), the OpenWA organization and project slugs from its URL (`/api/v1/orgs/<org>/projects/<project>`), the device id (`dev_…`) and the device key. Save. The API call is `CreateConnection`; it takes no signing secret.
2. Copy the **webhook URL** the page shows (`<http.baseURL>/hooks/openwa/<connection id>`).
3. In OpenWA, create a webhook endpoint with that URL, subscribed to the event `message.matched`. OpenWA shows a signing secret (`whsec_…`).
4. Back in oagent, paste the secret under **Paste the signing secret from OpenWA** (`SetConnectionSecret`). Until it is set, `secret_set` is false and oagent answers every delivery as if the connection did not exist.
5. Send the device a WhatsApp message. The status moves from **waiting** to **connected** on the first verified delivery.

| Status                                          | Meaning                                           | Fix                                                                |
| ----------------------------------------------- | ------------------------------------------------- | ------------------------------------------------------------------ |
| waiting                                         | no verified delivery yet                          | check the webhook URL and event in OpenWA                          |
| waiting, with a recent "unsigned delivery" time | deliveries arrive but fail the signature          | paste the secret OpenWA shows for this endpoint                    |
| device mismatch                                 | a signed delivery names another device            | the connection's device id must match the OpenWA endpoint's device |
| key refused                                     | OpenWA answered 401 to a send, photo or read call | replace the device key                                             |

## Turn a connection off

1. On the agent's **Channels** page, set **Answer messages on this number** to **Off** and confirm (`UpdateConnection` with `enabled: false`).
2. Set it back to **On** to answer again.

While off, messages are refused and not stored; senders get no reply and won't appear in Conversations. To see what was refused, check webhook deliveries in OpenWA.

## Rotate the signing secret

1. Rotate the endpoint's secret in OpenWA. During its rotation OpenWA signs with both secrets.
2. Paste the new secret in oagent with **Stop accepting the previous secret now** off. The old one stays valid (`secondary_set` is true).
3. Once OpenWA has dropped the old secret, paste the same new secret again with that box on (`drop_previous: true`) to retire it.

## What oagent does with a message

| Step         | Behaviour                                                                                                                                                                                        |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| verify       | `X-Openwa-Signature` against the stored secrets, timestamp within ±5 min, the envelope type `message.matched`, the device id                                                                     |
| limits       | per client IP before anything is read (`rateLimit.ingest.perIPPerMinute`, default 600), then per connection (`perConnectionPerMinute` 120, `burst` 30); over either gives 429 with `Retry-After` |
| dedupe       | each delivery id is taken once; ids older than 48 h are pruned by the `ingest-prune` job                                                                                                         |
| turn         | messages wait for the connection's quiet window (3 s, at most 10 s), then run as one turn in WhatsApp time order; a message older than `channel.maxMessageAge` (30 m) is skipped                 |
| photos       | up to 3 per turn, 5 MiB each, fetched from OpenWA for that turn only; a model without vision gets the caption and the person gets the photo set reply                                            |
| voice        | a voice-only DM gets the voice set reply; a group stays silent                                                                                                                                   |
| reply        | the agent's last answer, formatted for WhatsApp and split into at most 3 parts; groups quote the message answered. A failed send is retried by the `channel-reply` worker for up to 10 minutes   |
| read receipt | the last message of the turn is marked read when the turn starts                                                                                                                                 |
