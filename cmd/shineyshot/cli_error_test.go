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

func TestParseDrawClipboardRequiresOutput(t *testing.T) {
	_, err := parseDrawCmd([]string{"-from-clipboard", "line", "0", "0", "1", "1"}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if want := "output file is required when reading from the clipboard"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to mention %q, got %v", want, err)
	}
}

func TestParseAnnotateClipboardCaptureError(t *testing.T) {
	_, err := parseAnnotateCmd([]string{"-from-clipboard", "capture", "screen"}, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if want := "not supported"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to mention %q, got %v", want, err)
	}
}

func TestFileCaptureRejectsClipboard(t *testing.T) {
	r := &root{program: "shineyshot"}
	cmd, err := parseFileCmd([]string{"-file", "out.png", "-from-clipboard", "capture", "screen"}, r)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected error")
	} else if want := "-from-clipboard cannot be used"; !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to mention %q, got %v", want, err)
	}
}
