package capabilities_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/capabilities"
)

func TestFrom(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg  config.Config
		want capabilities.Capabilities
	}{
		"selfhosted local": {
			cfg:  config.Config{Mode: config.ModeSelfhosted, Auth: config.AuthConfig{Mode: config.AuthModeLocal}},
			want: capabilities.Capabilities{LocalLogin: true, PersonalKeys: true},
		},
		"selfhosted local with genesis": {
			cfg: config.Config{Mode: config.ModeSelfhosted, Auth: config.AuthConfig{
				Mode: config.AuthModeLocal, Genesis: config.GenesisConfig{Email: "root@example.com", Password: "long-enough"},
			}},
			want: capabilities.Capabilities{LocalLogin: true, PersonalKeys: true, EnvGenesis: true},
		},
		"selfhosted basic": {
			cfg:  config.Config{Mode: config.ModeSelfhosted, Auth: config.AuthConfig{Mode: config.AuthModeBasic}},
			want: capabilities.Capabilities{BasicAuth: true},
		},
		"cloud": {
			cfg:  config.Config{Mode: config.ModeCloud, Auth: config.AuthConfig{Mode: config.AuthModeLocal}},
			want: capabilities.Capabilities{OrgCreation: true, PersonalKeys: true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, capabilities.From(&tc.cfg))
		})
	}
}
