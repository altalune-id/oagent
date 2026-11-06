package runtime

// DriverHTTP calls a runtime service over HTTP.
const DriverHTTP = "http"

// Config picks the runtime driver and where it listens. SECURITY: Token is a secret.
type Config struct {
	Driver string
	URL    string
	Token  string
}
