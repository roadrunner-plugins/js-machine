package jsmachine

import "fmt"

// Config holds plugin configuration
type Config struct {
	// Pool configuration
	PoolSize       int `mapstructure:"pool_size"`
	MaxMemoryMB    int `mapstructure:"max_memory_mb"`
	DefaultTimeout int `mapstructure:"default_timeout_ms"`
}

// InitDefaults sets default configuration values
func (c *Config) InitDefaults() {
	if c.PoolSize == 0 {
		c.PoolSize = 4
	}
	if c.MaxMemoryMB == 0 {
		c.MaxMemoryMB = 512
	}
	if c.DefaultTimeout == 0 {
		c.DefaultTimeout = 30000
	}
}

// Validate ensures the configuration is valid
func (c *Config) Validate() error {
	if c.PoolSize < 1 {
		return fmt.Errorf("pool_size must be at least 1, got %d", c.PoolSize)
	}
	if c.PoolSize > 100 {
		return fmt.Errorf("pool_size cannot exceed 100, got %d", c.PoolSize)
	}
	if c.DefaultTimeout < 100 {
		return fmt.Errorf("default_timeout_ms must be at least 100ms, got %d", c.DefaultTimeout)
	}
	if c.MaxMemoryMB < 64 {
		return fmt.Errorf("max_memory_mb must be at least 64MB, got %d", c.MaxMemoryMB)
	}
	return nil
}
