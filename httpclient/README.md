# httpclient

Builds outbound `*http.Client`s that are SSRF-filtered, traced and bounded by default.

## Usage

```go
c := httpclient.New(
	httpclient.WithAllowPrivateHosts(true), // trusted target, e.g. localhost
	httpclient.WithTimeout(30*time.Second),
	httpclient.WithRetry(httpclient.RetryPolicy{MaxAttempts: 3}),
)
resp, err := c.Get(url)
```

## Options

| Option                                | Effect                                            |
| ------------------------------------- | ------------------------------------------------- |
| `WithAllowPrivateHosts(bool)`         | allow private/loopback targets (default: refused) |
| `WithDialTimeout(d)`                  | per-dial timeout (default 10s)                    |
| `WithTimeout(d)` / `WithoutTimeout()` | whole-request bound (default 30s)                 |
| `WithResponseBodyLimit(n)`            | cap response body bytes (0 = unlimited)           |
| `WithRetry(RetryPolicy{...})`         | retrying transport; `MaxAttempts < 2` disables it |
| `WithOtel(bool)`                      | otelhttp instrumentation (default on)             |

## Notes

- Dials to private, loopback, link-local, multicast, CGNAT and reserved ranges are refused with `PrivateAddressError` unless `WithAllowPrivateHosts(true)` is set. A proxy disables the filter (it sees only the proxy address).
- Retries replay only idempotent methods; POST/PATCH need `RetryNonIdempotent`. Waits honor `Retry-After` and stop when the context budget can't cover the backoff.
- Errors: `IsPrivateAddressError`, `IsBodyTooLargeError`.
