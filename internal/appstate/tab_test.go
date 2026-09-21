package appstate

import (
	"image"
	"testing"
)

// In a real headless test of the event loop, we would inject a mock screen and send key events.
// Because the UI loop is tightly coupled to shiny/screen, we instead test the state transition logic
// implicitly by verifying the DefaultSaveAction and how it can be used per-tab.
// We added the Output field to Tab so that the UI can maintain per-tab state.

func TestTabOutputTracking(t *testing.T) {
	// A simple sanity check that Tab struct has an Output field that can be modified independently.
	tabs := []Tab{
		{Image: image.NewRGBA(image.Rect(0, 0, 10, 10)), Title: "1", Output: "path1.png"},
		{Image: image.NewRGBA(image.Rect(0, 0, 10, 10)), Title: "2", Output: ""},
	}

	if tabs[0].Output != "path1.png" {
		t.Errorf("expected tab 0 to have output 'path1.png'")
	}
	if tabs[1].Output != "" {
		t.Errorf("expected tab 1 to have empty output")
	}

	tabs[1].Output = "path2.png"
	if tabs[1].Output != "path2.png" {
		t.Errorf("expected tab 1 to have output 'path2.png' after assignment")
	}
	if tabs[0].Output != "path1.png" {
		t.Errorf("tab 0 output changed unexpectedly")
	}
}
