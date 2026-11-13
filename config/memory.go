package config

// MemoryBackendNative names the built-in Postgres memory backend.
const MemoryBackendNative = "native"

// MemoryConfig turns on the backends memory stores may use; none on means no store is created and no run remembers.
type MemoryConfig struct {
	Native MemoryNativeConfig `yaml:"native" mapstructure:"native"`
}

// MemoryNativeConfig turns on the built-in Postgres backend.
type MemoryNativeConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

// Backends lists the enabled backends.
func (m MemoryConfig) Backends() []string {
	var out []string
	if m.Native.Enabled {
		out = append(out, MemoryBackendNative)
	}
	return out
}
