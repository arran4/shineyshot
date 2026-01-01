package config

import (
	"github.com/example/shineyshot/internal/theme"
)

// Notify holds notification settings.
type Notify struct {
	Capture bool
	Save    bool
	Copy    bool
}

// Config holds the application configuration.
type Config struct {
	Theme   string
	SaveDir string
	Notify  Notify
	Themes  map[string]*theme.Theme
}

// New creates a new Config with defaults.
func New() *Config {
	return &Config{
		Theme: "default",
		Notify: Notify{
			Capture: false,
			Save:    false,
			Copy:    false,
		},
		Themes: make(map[string]*theme.Theme),
	}
}
