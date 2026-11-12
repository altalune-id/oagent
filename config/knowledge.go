package config

// KnowledgeBackendPgvector names the built-in vector store.
const KnowledgeBackendPgvector = "pgvector"

// KnowledgeConfig turns on the knowledge backends sources may use; none on means knowledge is off.
type KnowledgeConfig struct {
	Pgvector KnowledgePgvectorConfig `yaml:"pgvector" mapstructure:"pgvector"`
	// AllowPrivateHosts lets a production deploy reach an embeddings server on a private network; single-tenant installs only.
	AllowPrivateHosts bool `yaml:"allowPrivateHosts" mapstructure:"allowPrivateHosts"`
}

// KnowledgePgvectorConfig turns on the built-in pgvector store; the database needs the vector extension.
type KnowledgePgvectorConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

// Backends lists the enabled backends.
func (k KnowledgeConfig) Backends() []string {
	var out []string
	if k.Pgvector.Enabled {
		out = append(out, KnowledgeBackendPgvector)
	}
	return out
}
