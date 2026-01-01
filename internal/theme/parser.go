package theme

import (
	"bufio"
	"fmt"
	"image/color"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// Parse reads a theme definition from an io.Reader.
// The format is a simple key-value pair per line: Key: #RRGGBB or #RRGGBBAA
func Parse(r io.Reader) (*Theme, error) {
	t := Default() // Start with defaults
	scanner := bufio.NewScanner(r)

	val := reflect.ValueOf(t).Elem()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "Name" {
			t.Name = value
			continue
		}

		field := val.FieldByName(key)
		if !field.IsValid() {
			continue // Unknown field, ignore for forward compatibility
		}

		if field.Type() == reflect.TypeOf(color.RGBA{}) {
			col, err := parseColor(value)
			if err != nil {
				return nil, fmt.Errorf("invalid color for key %s: %w", key, err)
			}
			field.Set(reflect.ValueOf(col))
		}
	}

	return t, scanner.Err()
}

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
