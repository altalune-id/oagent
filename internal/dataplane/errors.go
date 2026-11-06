package dataplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/reqid"
)

type envelope struct {
	Code      apperror.Code     `json:"code"`
	Message   string            `json:"message"`
	RequestID string            `json:"request_id"`
	Meta      map[string]string `json:"meta,omitempty"`
}

var (
	errNoKey        = apperror.New(apperror.CodeUnauthenticated, "An API key is required")
	errBodyTooLarge = apperror.New(apperror.CodeValidation, "The request body is too large").
			WithMeta(map[string]string{"field": "body", "reason": string(apperror.FieldTooLong)})
)

type statusError struct {
	status     int
	retryAfter time.Duration
	err        error
}

func (e *statusError) Error() string { return e.err.Error() }

func (e *statusError) Unwrap() error { return e.err }

func withStatus(status int, err error) error { return &statusError{status: status, err: err} }

func retryLater(status int, err error) error {
	return &statusError{status: status, retryAfter: retryAfter, err: err}
}

func statusOf(code apperror.Code) int {
	switch code {
	case apperror.CodeRunIdempotencyConflict:
		return http.StatusUnprocessableEntity
	case apperror.CodeRunInProgress:
		return http.StatusConflict
	case apperror.CodeRunRuntimeUnavailable:
		return http.StatusServiceUnavailable
	}
	switch apperror.KindOf(code) {
	case apperror.KindInvalid:
		return http.StatusBadRequest
	case apperror.KindNotFound:
		return http.StatusNotFound
	case apperror.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperror.KindForbidden:
		return http.StatusForbidden
	case apperror.KindAlreadyExists, apperror.KindFailedPrecondition:
		return http.StatusConflict
	case apperror.KindResourceExhausted:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := 0
	if se, ok := errors.AsType[*statusError](err); ok {
		status = se.status
		if se.retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int((se.retryAfter+time.Second-1)/time.Second)))
		}
	}
	ae, ok := apperror.AsAppError(err)
	if !ok {
		ae = h.d.Unexpected(r.Context(), "dataplane: "+r.Pattern, err)
	}
	if status == 0 {
		status = statusOf(ae.Code())
	}
	id := reqid.FromHeader(r.Header)
	if id == "" {
		id = reqid.New()
	}
	w.Header().Set(reqid.Header, id)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Code: ae.Code(), Message: ae.Message(), RequestID: id, Meta: ae.Meta()})
}
