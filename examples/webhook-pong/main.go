// Command webhook-pong is a localhost webhook receiver that verifies X-Oagent-Signature and answers 200 PONG!!!.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	signatureScheme    = "v1="
	timestampTolerance = 5 * time.Minute
	maxBodyBytes       = 1 << 20
)

func main() {
	var addr, secret string
	flag.StringVar(&addr, "addr", envOr("ADDR", "127.0.0.1:9099"), "listen address")
	flag.StringVar(&secret, "secret", os.Getenv("OAGENT_WEBHOOK_SECRET"), "signing secret; when set, invalid signatures are rejected 401")
	flag.Parse()

	if secret == "" {
		log.Print("WARNING: no -secret set; signatures are not verified")
	}

	srv := &http.Server{Addr: addr, Handler: handler(secret), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("webhook-pong listening on http://%s", addr)
	log.Fatal(srv.ListenAndServe())
}

func handler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		_ = r.Body.Close()
		logDelivery(r, body)

		if secret != "" && !verify(secret, r.Header, body, time.Now()) {
			log.Print("  signature: INVALID -> 401")
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		if secret != "" {
			log.Print("  signature: ok")
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "PONG!!! %s\n", time.Now().Format(time.RFC3339Nano))
	}
}

func verify(secret string, h http.Header, body []byte, now time.Time) bool {
	ts := h.Get("X-Oagent-Timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if d := now.Sub(time.Unix(sec, 0)); d > timestampTolerance || d < -timestampTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := []byte(signatureScheme + hex.EncodeToString(mac.Sum(nil)))
	for got := range strings.FieldsSeq(h.Get("X-Oagent-Signature")) {
		if hmac.Equal([]byte(got), want) {
			return true
		}
	}
	return false
}

func logDelivery(r *http.Request, body []byte) {
	log.Printf("--- %s %s ---", r.Method, r.URL.Path) //nolint:gosec // G706: echoes the raw delivery for inspection
	for _, h := range []string{
		"Content-Type", "User-Agent",
		"X-Oagent-Event-Type", "X-Oagent-Event-Id", "X-Oagent-Delivery-Id", "X-Oagent-Timestamp",
	} {
		if v := r.Header.Get(h); v != "" {
			log.Printf("  %s: %q", h, v) //nolint:gosec // G706
		}
	}
	if sig := r.Header.Get("X-Oagent-Signature"); sig != "" {
		log.Print("  X-Oagent-Signature: (present)")
	}
	if len(body) > 0 {
		log.Printf("  body: %s", body) //nolint:gosec // G706
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
