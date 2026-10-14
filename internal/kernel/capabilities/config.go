package capabilities

// Config is the deployment posture the switches derive from.
type Config struct {
	Selfhosted bool
	Cloud      bool
	BasicAuth  bool
	// EnvGenesis is true when the first operator comes from the environment.
	EnvGenesis bool
}
