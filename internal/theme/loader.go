package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader handles loading themes from various sources.
type Loader struct {
	ConfigDir string
	SystemDir string
}

// NewLoader creates a new Loader with standard paths.
func NewLoader() *Loader {
	home, _ := os.UserHomeDir()
	return &Loader{
		ConfigDir: filepath.Join(home, ".config", "shineyshot", "themes"),
		SystemDir: "/usr/share/shineyshot/themes",
	}
}

// Load attempts to load a theme by name or path.
// Order:
// 1. If it's a file path that exists, load it.
// 2. Check embedded themes.
// 3. Check ConfigDir.
// 4. Check SystemDir.
// 5. Fallback to Default.
func (l *Loader) Load(name string) (*Theme, error) {
	if name == "" {
		return Default(), nil
	}

	// 1. File path
	if _, err := os.Stat(name); err == nil {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return Parse(f)
	}

	// Normalize name (ensure .theme extension for lookup if missing)
	filename := name
	if !strings.HasSuffix(filename, ".theme") {
		filename += ".theme"
	}

	// 2. Embedded
	if f, err := EmbeddedThemes.Open("defaults/" + filename); err == nil {
		defer f.Close()
		return Parse(f)
	}

	// 3. Config Dir
	configPath := filepath.Join(l.ConfigDir, filename)
	if _, err := os.Stat(configPath); err == nil {
		f, err := os.Open(configPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return Parse(f)
	}

	// 4. System Dir
	systemPath := filepath.Join(l.SystemDir, filename)
	if _, err := os.Stat(systemPath); err == nil {
		f, err := os.Open(systemPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return Parse(f)
	}

	return nil, fmt.Errorf("theme '%s' not found", name)
}
