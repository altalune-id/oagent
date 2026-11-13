package mcpserver

// The MCP call timeout range; zero on create stores RecommendedTimeoutMS and zero on update keeps the stored value.
const (
	RecommendedTimeoutMS = 10000
	MinTimeoutMS         = 1000
	MaxTimeoutMS         = 20000
)
