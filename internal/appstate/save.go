package appstate

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SaveAction defines the strategy for saving an image from the UI.
// Implementations may prompt the user, automatically choose a file path,
// or write to a specific configured path.
type SaveAction func(img *image.RGBA) (string, error)

// DefaultSaveAction provides a safe default save flow.
// If output is not empty, it writes to output.
// If output is empty, it attempts to generate a unique filename in XDG_PICTURES_DIR
// or the user's home directory.
func DefaultSaveAction(output string) SaveAction {
	return func(img *image.RGBA) (string, error) {
		if output != "" {
			return saveToPath(img, output)
		}

		dir, err := defaultSaveDir()
		if err != nil {
			return "", err
		}

		ts := time.Now().Format("20060102-150405")
		prefix := "shineyshot"
		base := fmt.Sprintf("%s-%s.png", prefix, ts)
		path := filepath.Join(dir, base)
		counter := 1
		for {
			if _, err := os.Stat(path); err == nil {
				path = filepath.Join(dir, fmt.Sprintf("%s-%s-%02d.png", prefix, ts, counter))
				counter++
				continue
			} else if !os.IsNotExist(err) {
				return "", err
			}
			break
		}

		return saveToPath(img, path)
	}
}

func defaultSaveDir() (string, error) {
	if dir := os.Getenv("XDG_PICTURES_DIR"); dir != "" {
		return expandUserPath(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Pictures"), nil
}

func expandUserPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		if trimmed := strings.TrimPrefix(p, "~/"); trimmed != p {
			return filepath.Join(home, trimmed), nil
		}
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p), nil
}

func saveToPath(img *image.RGBA, path string) (string, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := png.Encode(f, img); err != nil {
		if cerr := f.Close(); cerr != nil {
			return "", fmt.Errorf("encode image: %w (close error: %v)", err, cerr)
		}
		return "", err
	}
	return path, f.Close()
}
