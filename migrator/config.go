package migrator

// Config shapes how migrations render and run; empty Schema defaults to "public".
type Config struct {
	Schema      string
	TablePrefix string
	Role        string
}

func (c Config) withDefaults() Config {
	if c.Schema == "" {
		c.Schema = "public"
	}
	return c
}
