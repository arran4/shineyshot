package appstate

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	tmpDir := t.TempDir()
	t.Setenv("XDG_PICTURES_DIR", tmpDir)

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

type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func TestSaveToAutoPath_Collision(t *testing.T) {
	tmpDir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	mc := &mockClock{now: fixedTime}

	// First save should succeed and create shineyshot-20260101-120000.png
	path1, err := saveToAutoPath(img, tmpDir, defaultFS, mc)
	if err != nil {
		t.Fatalf("First save failed: %v", err)
	}
	expected1 := filepath.Join(tmpDir, "shineyshot-20260101-120000.png")
	if path1 != expected1 {
		t.Errorf("Expected path %s, got %s", expected1, path1)
	}

	// Write something to the file to verify it's not truncated
	os.WriteFile(path1, []byte("original data"), 0644)

	// Second save should collide and create shineyshot-20260101-120000-01.png
	path2, err := saveToAutoPath(img, tmpDir, defaultFS, mc)
	if err != nil {
		t.Fatalf("Second save failed: %v", err)
	}
	expected2 := filepath.Join(tmpDir, "shineyshot-20260101-120000-01.png")
	if path2 != expected2 {
		t.Errorf("Expected path %s, got %s", expected2, path2)
	}

	// Verify original file was preserved
	data, _ := os.ReadFile(path1)
	if string(data) != "original data" {
		t.Errorf("Original file was truncated or modified")
	}

	// Verify new file exists
	if _, err := os.Stat(path2); os.IsNotExist(err) {
		t.Errorf("Second file was not created")
	}
}
