// Package ingest is the inbound webhook surface: provider pushes addressed to one connection and signed for it.
package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/web/layout"
)

// DefaultBasePath is the mount prefix this surface reserves.
const DefaultBasePath = "/hooks"

// DefaultMaxBodyBytes bounds a delivery body when HandlerParams leaves the limit unset.
const DefaultMaxBodyBytes int64 = 1 << 20

// Connection is the account a delivery is addressed to, resolved from its path; Secrets come from storage, never the body.
type Connection struct {
	PublicID string
	Secrets  []string
	Value    any
}

// Resolver finds the connection a path names; found is false for one that does not exist.
type Resolver interface {
	Resolve(ctx context.Context, publicID string) (c Connection, found bool, err error)
}

// Verifier authenticates a delivery against the raw body received and the connection it is addressed to.
type Verifier interface {
	Verify(r *http.Request, body []byte, c Connection) error
}

// Provider binds a registered provider's resolver and verifier to the handler its verified deliveries reach.
type Provider struct {
	Resolver Resolver
	Verifier Verifier
	Handler  http.Handler
}

// HandlerParams configures NewHandler, with BasePath as the full mount prefix; a nil Limiter limits nothing.
type HandlerParams struct {
	BasePath     string
	Providers    map[string]Provider
	MaxBodyBytes int64
	Log          *slog.Logger
	// Limiter counts every delivery per client IP under IPRule, before the body is read or the connection looked up.
	Limiter          ratelimit.Limiter
	IPRule           ratelimit.Rule
	TrustedProxyHops int
}

// Handler serves the surface, reading a delivery body once, verifying it against its connection, and only then dispatching.
type Handler struct {
	mux       *http.ServeMux
	providers map[string]Provider
	maxBody   int64
	log       *slog.Logger
	limiter   ratelimit.Limiter
	ipRule    ratelimit.Rule
	hops      int
}

type connectionKey struct{}

// ConnectionFrom returns the verified connection a delivery's handler serves.
func ConnectionFrom(ctx context.Context) (Connection, bool) {
	c, ok := ctx.Value(connectionKey{}).(Connection)
	return c, ok
}

// NewHandler returns the surface handler, which refuses every delivery until a provider is registered.
func NewHandler(p HandlerParams) *Handler {
	base := strings.TrimRight(p.BasePath, "/")
	if base == "" {
		base = DefaultBasePath
	}
	maxBody := p.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxBodyBytes
	}
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	h := &Handler{
		mux: http.NewServeMux(), providers: maps.Clone(p.Providers), maxBody: maxBody, log: log,
		limiter: p.Limiter, ipRule: p.IPRule, hops: p.TrustedProxyHops,
	}
	h.mux.Handle(base+"/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteError(w, &UnverifiedError{})
	}))
	h.mux.Handle("POST "+base+"/{provider}/{connection}", h.perIP(http.HandlerFunc(h.deliver)))
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

// SECURITY: the signature is the whole authorization; an unknown provider, connection or bad signature answer alike.
func (h *Handler) deliver(w http.ResponseWriter, r *http.Request) {
	p, ok := h.providers[r.PathValue("provider")]
	if !ok || p.Resolver == nil || p.Verifier == nil {
		WriteError(w, &UnverifiedError{})
		return
	}
	body, err := h.readBody(w, r)
	if err != nil {
		WriteError(w, err)
		return
	}
	publicID := r.PathValue("connection")
	conn, found, err := p.Resolver.Resolve(r.Context(), publicID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "ingest: resolve connection",
			slog.String("provider", r.PathValue("provider")), slog.String("err", err.Error()))
		WriteError(w, &UnavailableError{})
		return
	}
	if !found {
		WriteError(w, &UnverifiedError{})
		return
	}
	if err := p.Verifier.Verify(r, body, conn); err != nil {
		h.log.WarnContext(r.Context(), "ingest: delivery failed verification",
			slog.String("provider", r.PathValue("provider")), slog.String("connection", publicID), slog.String("err", err.Error()))
		WriteError(w, &UnverifiedError{})
		return
	}
	if p.Handler == nil {
		WriteError(w, &NoHandlerError{})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	p.Handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), connectionKey{}, conn)))
}

// SECURITY: keyed on the client IP only, so every id shares the caller's bucket; a store failure fails closed.
func (h *Handler) perIP(next http.Handler) http.Handler {
	if h.limiter == nil || h.ipRule.Name == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err := h.limiter.Allow(r.Context(), ratelimit.Key(h.ipRule, ratelimit.IPSubject(layout.ClientIP(r, h.hops))), h.ipRule)
		if err != nil {
			h.log.ErrorContext(r.Context(), "ingest: rate limit", slog.String("err", err.Error()))
			WriteError(w, &UnavailableError{})
			return
		}
		if !res.Allowed {
			WriteError(w, &RateLimitedError{RetryAfterSeconds: max(res.RetryAfterSeconds(), 1)})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBody))
	if err == nil {
		return body, nil
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return nil, &PayloadTooLargeError{}
	}
	return nil, &BadRequestError{}
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError answers a refused delivery with its status and code. SECURITY: the body names the outcome only; the cause stays in the log.
func WriteError(w http.ResponseWriter, err error) {
	status, code := statusFor(err)
	if limited, ok := errors.AsType[*RateLimitedError](err); ok {
		w.Header().Set("Retry-After", strconv.Itoa(limited.RetryAfterSeconds))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Code: code, Message: code})
}

func statusFor(err error) (status int, code string) {
	switch {
	case IsUnverifiedError(err):
		return http.StatusUnauthorized, "unauthorized"
	case IsPayloadTooLargeError(err):
		return http.StatusRequestEntityTooLarge, "payload_too_large"
	case IsBadRequestError(err):
		return http.StatusBadRequest, "bad_request"
	case IsUnavailableError(err):
		return http.StatusServiceUnavailable, "unavailable"
	case IsRateLimitedError(err):
		return http.StatusTooManyRequests, "rate_limited"
	}
	return http.StatusInternalServerError, "internal"
}

// IPRule limits one client IP to perMinute deliveries a minute, all at once at most.
func IPRule(perMinute int) ratelimit.Rule {
	return ratelimit.Rule{Name: "ingest-ip", Rate: perMinute, Per: time.Minute, Burst: perMinute}
}
