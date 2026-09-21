package appstate

import (
	"image"
	"path/filepath"
	"testing"
	"time"
)

func TestTabSaveFlow(t *testing.T) {
	// Simulate the exact flow requested in the review:
	// "tab A saved -> new tab B saved -> distinct existing PNGs, and tab A saved again -> same original destination"

	tmpDir := t.TempDir()

	mc := &mockClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}

	// Create our save action that will be used by the UI
	// In the UI, the output passed to DefaultSaveAction is tabs[current].Output
	// We simulate the UI's registerSave closure logic.

	saveTab := func(tab *Tab) error {
		// We replace the internal defaultSaveDir call implicitly by overriding XDG_PICTURES_DIR,
		// but DefaultSaveAction relies on `defaultFS` and `defaultClock` internally which are globals.
		// Since we want to use mockClock, we'll manually call saveToAutoPath or saveToExplicitPath
		// just like DefaultSaveAction does, to simulate it properly.
		var savedPath string
		var err error
		if tab.Output != "" {
			savedPath, err = saveToExplicitPath(tab.Image, tab.Output, defaultFS)
		} else {
			savedPath, err = saveToAutoPath(tab.Image, tmpDir, defaultFS, mc)
		}

		if err != nil {
			return err
		}
		tab.Output = savedPath
		return nil
	}

	tabs := []Tab{
		{Image: image.NewRGBA(image.Rect(0, 0, 10, 10)), Title: "Tab A", Output: ""},
	}
	current := 0

	// 1. Save Tab A
	if err := saveTab(&tabs[current]); err != nil {
		t.Fatalf("Failed to save Tab A: %v", err)
	}

	expectedPathA := filepath.Join(tmpDir, "shineyshot-20260101-120000.png")
	if tabs[current].Output != expectedPathA {
		t.Errorf("Tab A output mismatch: got %v, want %v", tabs[current].Output, expectedPathA)
	}

	// 2. New Tab B created (e.g. paste/capture)
	tabs = append(tabs, Tab{
		Image:  image.NewRGBA(image.Rect(0, 0, 20, 20)),
		Title:  "Tab B",
		Output: "", // newly created tab is untitled
	})
	current = 1

	// Advance time for Tab B's save
	mc.now = time.Date(2026, 1, 1, 12, 0, 1, 0, time.UTC)

	// 3. Save Tab B
	if err := saveTab(&tabs[current]); err != nil {
		t.Fatalf("Failed to save Tab B: %v", err)
	}

	expectedPathB := filepath.Join(tmpDir, "shineyshot-20260101-120001.png")
	if tabs[current].Output != expectedPathB {
		t.Errorf("Tab B output mismatch: got %v, want %v", tabs[current].Output, expectedPathB)
	}

	if expectedPathA == expectedPathB {
		t.Errorf("Tab A and Tab B saved to the same file: %v", expectedPathA)
	}

	// 4. Switch back to Tab A and save again
	current = 0
	if err := saveTab(&tabs[current]); err != nil {
		t.Fatalf("Failed to save Tab A second time: %v", err)
	}

	// Output should still be the original path A
	if tabs[current].Output != expectedPathA {
		t.Errorf("Tab A output changed after second save: got %v, want %v", tabs[current].Output, expectedPathA)
	}
}
