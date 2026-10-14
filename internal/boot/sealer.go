package boot

import (
	"altalune.id/oagent/config"
	"altalune.id/oagent/sealer"
)

// NOTE: a missing or invalid key yields a Disabled sealer, not a boot failure.
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
