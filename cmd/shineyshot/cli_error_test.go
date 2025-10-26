package main

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/example/shineyshot/internal/capture"
)

func TestSnapshotRunCaptureError(t *testing.T) {
	original := captureScreenshotFn
	sentinel := errors.New("boom")
	captureScreenshotFn = func(string, capture.CaptureOptions) (*image.RGBA, error) { return nil, sentinel }
	t.Cleanup(func() { captureScreenshotFn = original })

	cmd := &snapshotCmd{mode: "screen", stdout: true}
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected error")
	} else {
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected wrapped error, got %v", err)
		}
		if want := "failed to capture screen"; !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestAnnotateRunCaptureError(t *testing.T) {
	original := captureScreenshotFn
	sentinel := errors.New("denied")
	captureScreenshotFn = func(string, capture.CaptureOptions) (*image.RGBA, error) { return nil, sentinel }
	t.Cleanup(func() { captureScreenshotFn = original })

	cmd := &annotateCmd{action: "capture", target: "screen"}
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected error")
	} else {
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected wrapped error, got %v", err)
		}
		if want := "failed to capture screen"; !strings.Contains(err.Error(), want) {
			t.Fatalf("expected message context, got %v", err)
		}
	}
}
