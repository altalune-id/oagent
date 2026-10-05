package boot

import (
	"altalune.id/auth-server/config"
	"altalune.id/auth-server/internal/kernel"
	"altalune.id/auth-server/internal/org"
)

// Services is every store and service the composition root wires.
type Services struct {
	OrgStore org.Store
	Orgs     *org.Service
}

func buildServices(k kernel.Kernel, cfg config.Config) *Services {
	orgStore := org.NewStore(k.Pool, cfg.DB.Schema, cfg.DB.TablePrefix)
	return &Services{
		OrgStore: orgStore,
		Orgs:     org.NewService(orgStore, k.Reporter.Unexpected),
	}
}
