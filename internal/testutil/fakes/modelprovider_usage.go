package fakes

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/modelprovider"
)

// ModelProviderUsage is a modelprovider.Usage test double; Fn, when set, answers instead of Uses.
type ModelProviderUsage struct {
	Err  error
	Uses map[uuid.UUID]modelprovider.ProviderUse
	Fn   func(ctx context.Context, providerID uuid.UUID) (modelprovider.ProviderUse, error)
}

var _ modelprovider.Usage = (*ModelProviderUsage)(nil)

// ProviderUse implements modelprovider.Usage.
func (f *ModelProviderUsage) ProviderUse(ctx context.Context, providerID uuid.UUID) (modelprovider.ProviderUse, error) {
	if f.Fn != nil {
		return f.Fn(ctx, providerID)
	}
	return f.Uses[providerID], f.Err
}
