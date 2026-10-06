package boot

import (
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/org"
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
