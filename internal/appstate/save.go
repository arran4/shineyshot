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

// fileSystem defines the file operations seam for testing
type fileSystem interface {
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Create(name string) (*os.File, error)
	MkdirAll(path string, perm os.FileMode) error
}

type osFS struct{}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (osFS) Create(name string) (*os.File, error) {
	return os.Create(name)
}

func (osFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// defaultFS is the system filesystem wrapper used in production.
var defaultFS fileSystem = osFS{}

// clock defines the time operations seam for testing
type clock interface {
	Now() time.Time
}

type osClock struct{}

func (osClock) Now() time.Time { return time.Now() }

var defaultClock clock = osClock{}

// DefaultSaveAction provides a safe default save flow.
// If output is not empty, it writes to output.
// If output is empty, it attempts to generate a unique filename in XDG_PICTURES_DIR
// or the user's home directory atomically.
func DefaultSaveAction(output string) SaveAction {
	return func(img *image.RGBA) (string, error) {
		if output != "" {
			return saveToExplicitPath(img, output, defaultFS)
		}

		dir, err := defaultSaveDir()
		if err != nil {
			return "", err
		}

		return saveToAutoPath(img, dir, defaultFS, defaultClock)
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

func saveToExplicitPath(img *image.RGBA, path string, fs fileSystem) (string, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := fs.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	f, err := fs.Create(path)
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

func saveToAutoPath(img *image.RGBA, dir string, fs fileSystem, c clock) (string, error) {
	if dir != "" && dir != "." {
		if err := fs.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}

	ts := c.Now().Format("20060102-150405")
	prefix := "shineyshot"
	base := fmt.Sprintf("%s-%s.png", prefix, ts)
	path := filepath.Join(dir, base)
	counter := 1

	for {
		f, err := fs.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
		if err != nil {
			if os.IsExist(err) {
				path = filepath.Join(dir, fmt.Sprintf("%s-%s-%02d.png", prefix, ts, counter))
				counter++
				continue
			}
			return "", err
		}

		// File created successfully
		if err := png.Encode(f, img); err != nil {
			if cerr := f.Close(); cerr != nil {
				return "", fmt.Errorf("encode image: %w (close error: %v)", err, cerr)
			}
			return "", err
		}
		return path, f.Close()
	}
}
