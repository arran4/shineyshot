package skill

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type LocalSource struct {
	path string
}

func NewLocalSource(path string) *LocalSource {
	return &LocalSource{path: path}
}

func (s *LocalSource) Resolve(ctx context.Context) (fs.FS, string, error) {
	absPath, err := filepath.Abs(s.path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to stat local source: %w", err)
	}

	if !stat.IsDir() {
		return nil, "", fmt.Errorf("local source must be a directory")
	}

	// Ensure SKILL.md exists
	skillMdPath := filepath.Join(absPath, "SKILL.md")
	_, err = os.Stat(skillMdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("SKILL.md not found in %s", s.path)
		}
		return nil, "", fmt.Errorf("failed to stat SKILL.md: %w", err)
	}

	return os.DirFS(absPath), "local", nil
}

func (s *LocalSource) CanonicalName() string {
	abs, err := filepath.Abs(s.path)
	if err != nil {
		return s.path
	}
	return abs
}
