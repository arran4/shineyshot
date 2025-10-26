package main

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/example/shineyshot/internal/capture"
)

func TestSnapshotRunCaptureError(t *testing.T) {
	restoreShot := capture.SetScreenshotProviderForTests(func(bool) (*image.RGBA, error) {
		return nil, errors.New("portal offline")
	})
	t.Cleanup(restoreShot)

	cmd := &snapshotCmd{mode: "screen", stdout: true}
	if err := cmd.Run(); err == nil || err.Error() == "" || !containsAll(err.Error(), []string{"snapshot screen", "capture screen", "portal offline"}) {
		t.Fatalf("expected wrapped capture error, got %v", err)
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func TestAnnotateCaptureError(t *testing.T) {
	restoreShot := capture.SetScreenshotProviderForTests(func(bool) (*image.RGBA, error) {
		return nil, errors.New("dbus busy")
	})
	t.Cleanup(restoreShot)

	cmd := &annotateCmd{action: "capture", target: "screen", root: &root{}}
	if err := cmd.Run(); err == nil || !containsAll(err.Error(), []string{"annotate capture screen", "dbus busy"}) {
		t.Fatalf("expected annotate capture error, got %v", err)
	}
}

func TestAnnotateOpenError(t *testing.T) {
	cmd := &annotateCmd{action: "open", file: "missing.png", root: &root{}}
	if err := cmd.Run(); err == nil || !strings.Contains(err.Error(), "open missing.png") {
		t.Fatalf("expected open error context, got %v", err)
	}
}
