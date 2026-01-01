package config

import (
	"bufio"
	"fmt"
	"image/color"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/example/shineyshot/internal/theme"
)

// Parse reads configuration from an io.Reader.
func Parse(r io.Reader) (*Config, error) {
	cfg := New()
	scanner := bufio.NewScanner(r)

	// Context for parsing
	var currentSection string
	var currentTheme *theme.Theme

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Handle Sections
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			currentTheme = nil

			if strings.HasPrefix(currentSection, "theme.") {
				themeName := strings.TrimPrefix(currentSection, "theme.")
				// Start with defaults so missing keys are fine
				currentTheme = theme.Default()
				currentTheme.Name = themeName
				cfg.Themes[themeName] = currentTheme
			}
			continue
		}

		// Parse Key = Value or Key: Value
		var parts []string
		if strings.Contains(line, "=") {
			parts = strings.SplitN(line, "=", 2)
		} else if strings.Contains(line, ":") {
			parts = strings.SplitN(line, ":", 2)
		} else {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Remove quotes if present
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = value[1 : len(value)-1]
		}

		if currentTheme != nil {
			// Parsing a theme definition
			if err := setThemeField(currentTheme, key, value); err != nil {
				return nil, fmt.Errorf("error in section [%s]: %w", currentSection, err)
			}
		} else if currentSection == "notify" {
			if err := setNotifyField(&cfg.Notify, key, value); err != nil {
				return nil, fmt.Errorf("error in section [notify]: %w", err)
			}
		} else if currentSection == "" {
			// Root section
			if err := setRootField(cfg, key, value); err != nil {
				return nil, fmt.Errorf("error in root section: %w", err)
			}
		}
	}

	return cfg, scanner.Err()
}

func setRootField(cfg *Config, key, value string) error {
	switch strings.ToLower(key) {
	case "theme":
		cfg.Theme = value
	case "save_dir":
		cfg.SaveDir = value
	}
	return nil
}

func setNotifyField(n *Notify, key, value string) error {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("invalid boolean for key %s: %w", key, err)
	}
	switch strings.ToLower(key) {
	case "capture":
		n.Capture = b
	case "save":
		n.Save = b
	case "copy":
		n.Copy = b
	}
	return nil
}

func setThemeField(t *theme.Theme, key, value string) error {
	if strings.EqualFold(key, "Name") {
		t.Name = value
		return nil
	}

	val := reflect.ValueOf(t).Elem()

	// Case-insensitive field lookup
	typ := val.Type()
	var fieldName string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if strings.EqualFold(f.Name, key) {
			fieldName = f.Name
			break
		}
	}

	if fieldName == "" {
		return nil // Ignore unknown fields
	}

	field := val.FieldByName(fieldName)
	if !field.IsValid() {
		return nil // Should not happen if loop found it, but safety check
	}

	if field.Type() == reflect.TypeOf(color.RGBA{}) {
		col, err := parseColor(value)
		if err != nil {
			return fmt.Errorf("invalid color for key %s: %w", key, err)
		}
		field.Set(reflect.ValueOf(col))
	}
	return nil
}

// parseColor parses a hex color string.
// Duplicated from internal/theme/parser.go as it is unexported there.
func parseColor(s string) (color.RGBA, error) {
	if !strings.HasPrefix(s, "#") {
		return color.RGBA{}, fmt.Errorf("color must start with #")
	}
	hex := strings.TrimPrefix(s, "#")
	if len(hex) == 6 {
		// #RRGGBB
		val, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{
			R: uint8(val >> 16),
			G: uint8((val >> 8) & 0xFF),
			B: uint8(val & 0xFF),
			A: 255,
		}, nil
	} else if len(hex) == 8 {
		// #RRGGBBAA
		val, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.RGBA{}, err
		}
		return color.RGBA{
			R: uint8(val >> 24),
			G: uint8((val >> 16) & 0xFF),
			B: uint8((val >> 8) & 0xFF),
			A: uint8(val & 0xFF),
		}, nil
	}
	return color.RGBA{}, fmt.Errorf("invalid hex length")
}
