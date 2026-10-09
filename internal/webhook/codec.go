package webhook

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
)

func prepareAttempt(tc tenant.Context, a Attempt) (Attempt, error) {
	if a.OrgID != tc.OrgID {
		// SECURITY: an attempt naming another org would land in that org's history.
		return Attempt{}, &InvalidAttemptError{Field: "OrgID", Reason: "does not match the tenant scope on ctx"}
	}
	if a.ProjectID != tc.ProjectID {
		return Attempt{}, &InvalidAttemptError{Field: "ProjectID", Reason: "does not match the tenant scope on ctx"}
	}
	if a.ID == uuid.Nil {
		a.ID = uuid.Must(uuid.NewV7())
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	a.CreatedAt = a.CreatedAt.UTC()
	a.Error = apperror.TruncateCause(a.Error, outbox.MaxCauseLen)
	return BoundResponse(a), nil
}

func marshalEventTypes(types []events.Type) (string, error) {
	if types == nil {
		types = []events.Type{}
	}
	b, err := json.Marshal(types)
	if err != nil {
		return "", fmt.Errorf("webhook: marshal event types: %w", err)
	}
	return string(b), nil
}

func unmarshalEventTypes(raw string) ([]events.Type, error) {
	var types []events.Type
	if err := json.Unmarshal([]byte(raw), &types); err != nil {
		return nil, fmt.Errorf("webhook: unmarshal event types: %w", err)
	}
	return types, nil
}

func marshalHeaders(hs []Header) (string, error) {
	stored := make([]Header, len(hs))
	copy(stored, hs)
	b, err := json.Marshal(stored)
	if err != nil {
		return "", fmt.Errorf("webhook: marshal response headers: %w", err)
	}
	return string(b), nil
}

func unmarshalHeaders(raw string) ([]Header, error) {
	var hs []Header
	if err := json.Unmarshal([]byte(raw), &hs); err != nil {
		return nil, fmt.Errorf("webhook: unmarshal response headers: %w", err)
	}
	return hs, nil
}
