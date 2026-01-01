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

func TestCircular(t *testing.T) {
	input := `theme = dark
save_dir = /home/user/shots

[notify]
capture = true
save = true
copy = false

[theme.custom]
Name = custom
Background = #000000
Foreground = #FFFFFF
`
	// 1. Parse initial input
	cfg, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Initial parse failed: %v", err)
	}

	// 2. Generate string representation
	generated := cfg.String()

	// 3. Parse generated string
	cfg2, err := Parse(strings.NewReader(generated))
	if err != nil {
		t.Fatalf("Circular parse failed: %v", err)
	}

	// 4. Compare relevant fields
	if cfg.Theme != cfg2.Theme {
		t.Errorf("Theme mismatch: %q vs %q", cfg.Theme, cfg2.Theme)
	}
	if cfg.SaveDir != cfg2.SaveDir {
		t.Errorf("SaveDir mismatch: %q vs %q", cfg.SaveDir, cfg2.SaveDir)
	}
	if cfg.Notify != cfg2.Notify {
		t.Errorf("Notify mismatch: %+v vs %+v", cfg.Notify, cfg2.Notify)
	}

	// Check theme persistence
	t1 := cfg.Themes["custom"]
	t2 := cfg2.Themes["custom"]
	if t1 == nil || t2 == nil {
		t.Fatalf("Custom theme missing in one config")
	}
	if t1.Background != t2.Background {
		t.Errorf("Theme background mismatch: %v vs %v", t1.Background, t2.Background)
	}
}
