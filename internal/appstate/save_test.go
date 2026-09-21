package appstate

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSaveAction_WithOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "explicit.png")

	action := DefaultSaveAction(outPath)
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	savedPath, err := action(img)
	if err != nil {
		t.Fatalf("DefaultSaveAction failed: %v", err)
	}

	if savedPath != outPath {
		t.Errorf("expected savedPath to be %s, got %s", outPath, savedPath)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("file was not created at %s", outPath)
	}
}

func TestDefaultSaveAction_EmptyOutput(t *testing.T) {
	// Set XDG_PICTURES_DIR so we don't pollute the real home directory
	tmpDir := t.TempDir()
	os.Setenv("XDG_PICTURES_DIR", tmpDir)
	defer os.Unsetenv("XDG_PICTURES_DIR")

	action := DefaultSaveAction("")
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	savedPath, err := action(img)
	if err != nil {
		t.Fatalf("DefaultSaveAction failed: %v", err)
	}

	if !strings.HasPrefix(savedPath, tmpDir) {
		t.Errorf("expected savedPath to be inside %s, got %s", tmpDir, savedPath)
	}

	if _, err := os.Stat(savedPath); os.IsNotExist(err) {
		t.Errorf("file was not created at %s", savedPath)
	}
}
