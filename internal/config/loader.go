package config

import (
	"os"
	"path/filepath"
)

// Loader handles loading the configuration.
type Loader struct {
	Version      string // Build version, used to determine dev mode
	OverridePath string // Set at compile time if needed
}

// NewLoader creates a new Loader.
func NewLoader(version string, overridePath string) *Loader {
	return &Loader{
		Version:      version,
		OverridePath: overridePath,
	}
}

// Load attempts to load the configuration.
func (l *Loader) Load() (*Config, error) {
	path := l.GetConfigPath()
	if path == "" {
		return New(), nil // No config file found, return defaults
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return Parse(f)
}

// GetConfigPath returns the path to the configuration file, or empty string if not found.
func (l *Loader) GetConfigPath() string {
	// 1. Variable override path
	if l.OverridePath != "" {
		if _, err := os.Stat(l.OverridePath); err == nil {
			return l.OverridePath
		}
	}

	// 2. Local run directory (dev mode)
	if l.Version == "dev" {
		wd, _ := os.Getwd()
		localPath := filepath.Join(wd, ".shineyshotrc")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
	}

	// 3. XDG Config Path
	home, _ := os.UserHomeDir()
	xdgPath := filepath.Join(home, ".config", "shineyshot", "config.rc")
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath
	}

	// Fallback names
	xdgPath = filepath.Join(home, ".config", "shineyshot", "shineyshot.rc")
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath
	}

	return ""
}
