package config

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

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
		Theme: "", // Default to empty to allow fallback to Env/Default
		Notify: Notify{
			Capture: false,
			Save:    false,
			Copy:    false,
		},
		Themes: make(map[string]*theme.Theme),
	}
}

// String implements fmt.Stringer and returns the configuration in RC format.
func (c *Config) String() string {
	var sb strings.Builder

	// Root section
	if c.Theme != "" {
		fmt.Fprintf(&sb, "theme = %s\n", c.Theme)
	}
	if c.SaveDir != "" {
		fmt.Fprintf(&sb, "save_dir = %s\n", c.SaveDir)
	}
	sb.WriteString("\n")

	// Notify section
	sb.WriteString("[notify]\n")
	fmt.Fprintf(&sb, "capture = %v\n", c.Notify.Capture)
	fmt.Fprintf(&sb, "save = %v\n", c.Notify.Save)
	fmt.Fprintf(&sb, "copy = %v\n", c.Notify.Copy)
	sb.WriteString("\n")

	// Themes sections
	// Sort keys for deterministic output
	var themeNames []string
	for name := range c.Themes {
		themeNames = append(themeNames, name)
	}
	sort.Strings(themeNames)

	for _, name := range themeNames {
		t := c.Themes[name]
		fmt.Fprintf(&sb, "[theme.%s]\n", name)
		fmt.Fprintf(&sb, "Name: %s\n", t.Name)
		fmt.Fprintf(&sb, "Background: %s\n", toHex(t.Background))
		fmt.Fprintf(&sb, "Foreground: %s\n", toHex(t.Foreground))
		fmt.Fprintf(&sb, "ToolbarBackground: %s\n", toHex(t.ToolbarBackground))
		fmt.Fprintf(&sb, "TabBackground: %s\n", toHex(t.TabBackground))
		fmt.Fprintf(&sb, "TabActive: %s\n", toHex(t.TabActive))
		fmt.Fprintf(&sb, "TabHover: %s\n", toHex(t.TabHover))
		fmt.Fprintf(&sb, "TabText: %s\n", toHex(t.TabText))
		fmt.Fprintf(&sb, "TabTextActive: %s\n", toHex(t.TabTextActive))
		fmt.Fprintf(&sb, "TabTextHover: %s\n", toHex(t.TabTextHover))
		fmt.Fprintf(&sb, "ButtonBackground: %s\n", toHex(t.ButtonBackground))
		fmt.Fprintf(&sb, "ButtonBackgroundHover: %s\n", toHex(t.ButtonBackgroundHover))
		fmt.Fprintf(&sb, "ButtonBackgroundPress: %s\n", toHex(t.ButtonBackgroundPress))
		fmt.Fprintf(&sb, "ButtonText: %s\n", toHex(t.ButtonText))
		fmt.Fprintf(&sb, "ButtonTextHover: %s\n", toHex(t.ButtonTextHover))
		fmt.Fprintf(&sb, "ButtonTextPress: %s\n", toHex(t.ButtonTextPress))
		fmt.Fprintf(&sb, "ButtonBorder: %s\n", toHex(t.ButtonBorder))
		fmt.Fprintf(&sb, "CheckerLight: %s\n", toHex(t.CheckerLight))
		fmt.Fprintf(&sb, "CheckerDark: %s\n", toHex(t.CheckerDark))
		sb.WriteString("\n")
	}

	return sb.String()
}

func toHex(c interface{ RGBA() (r, g, b, a uint32) }) string {
	if rgba, ok := c.(color.RGBA); ok {
		if rgba.A == 255 {
			return fmt.Sprintf("#%02X%02X%02X", rgba.R, rgba.G, rgba.B)
		}
		return fmt.Sprintf("#%02X%02X%02X%02X", rgba.R, rgba.G, rgba.B, rgba.A)
	}

	// Fallback for non-color.RGBA types (though unlikely in this app's context)
	r, g, b, a := c.RGBA()
	if a == 0 {
		return "#00000000"
	}
	r8 := uint8(r >> 8)
	g8 := uint8(g >> 8)
	b8 := uint8(b >> 8)
	a8 := uint8(a >> 8)

	if a8 == 255 {
		return fmt.Sprintf("#%02X%02X%02X", r8, g8, b8)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", r8, g8, b8, a8)
}
