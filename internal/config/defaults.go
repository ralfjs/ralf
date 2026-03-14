package config

// DefaultConfig returns an empty configuration with no rules.
func DefaultConfig() *Config {
	return &Config{
		Rules: make(map[string]RuleConfig),
	}
}
