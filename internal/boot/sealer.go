package boot

import (
	"altalune.id/oagent/config"
	"altalune.id/oagent/sealer"
)

// buildSealer returns a working sealer from the configured key, or a Disabled one when it is absent or invalid.
func buildSealer(cfg config.SecretsConfig) sealer.Sealer {
	key, err := sealer.ParseKey(cfg.EncryptionKey)
	if err != nil {
		return sealer.Disabled()
	}
	sl, err := sealer.New(key)
	if err != nil {
		return sealer.Disabled()
	}
	return sl
}
