package idp

// Config represents the configuration for the engine.
type Config struct {
	Command     string
	Environment map[string]string
	WorkDir     string
	User        string
	Shell       string
	// ReadOnly means none-interactive for terminal, which is used for show log, like top
	ReadOnly bool

	Server       string
	ClientID     string
	ClientSecret string

	// Custom Command Runner ID
	ID string

	// AllowedSystemEnvKeys is the allowed system environment keys, which will be inherited to the command
	AllowedSystemEnvKeys []string
}
