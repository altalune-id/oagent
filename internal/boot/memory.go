package boot

import (
	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/memory"
	"altalune.id/oagent/internal/memory/native"
)

// NOTE: native is built even with its backend off, so stored facts stay erasable; a new driver adds one entry, built only when enabled.
func memoryDrivers(cfg config.Config, k kernel.Kernel) map[memory.Backend]memory.Facts {
	return map[memory.Backend]memory.Facts{
		memory.BackendNative: native.New(k.Pool, cfg.Postgres.Schema, cfg.Postgres.TablePrefix),
	}
}

func memoryBackends(cfg config.Config) []memory.Backend {
	backends := cfg.Memory.Backends()
	out := make([]memory.Backend, 0, len(backends))
	for _, b := range backends {
		out = append(out, memory.Backend(b))
	}
	return out
}
