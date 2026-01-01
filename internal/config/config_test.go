package config

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := `
theme = my_custom_theme
save_dir = /tmp/screens

[notify]
capture = true
save = false
copy = true

[theme.my_custom_theme]
Background = #111111
Foreground = #FFFFFF
`
	r := strings.NewReader(input)
	cfg, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.Theme != "my_custom_theme" {
		t.Errorf("Expected theme 'my_custom_theme', got '%s'", cfg.Theme)
	}

	if cfg.SaveDir != "/tmp/screens" {
		t.Errorf("Expected save_dir '/tmp/screens', got '%s'", cfg.SaveDir)
	}

	if !cfg.Notify.Capture {
		t.Error("Expected notify.capture to be true")
	}
	if cfg.Notify.Save {
		t.Error("Expected notify.save to be false")
	}
	if !cfg.Notify.Copy {
		t.Error("Expected notify.copy to be true")
	}

	theme, ok := cfg.Themes["my_custom_theme"]
	if !ok {
		t.Fatal("Expected theme 'my_custom_theme' to be loaded")
	}

	if theme.Background.R != 0x11 || theme.Background.G != 0x11 || theme.Background.B != 0x11 {
		t.Errorf("Unexpected Background color: %+v", theme.Background)
	}
}
