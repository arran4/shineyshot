package main

import (
	"flag"
	"testing"
)

func TestNotificationFlagsPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		configCapture bool
		configSave    bool
		configCopy    bool
		args          []string
		wantCapture   bool
		wantSave      bool
		wantCopy      bool
	}{
		{
			name:          "defaults (all false)",
			configCapture: false, configSave: false, configCopy: false,
			args:        []string{"version"},
			wantCapture: false, wantSave: false, wantCopy: false,
		},
		{
			name:          "config overrides defaults",
			configCapture: true, configSave: true, configCopy: true,
			args:        []string{"version"},
			wantCapture: true, wantSave: true, wantCopy: true,
		},
		{
			name:          "flags override config (enable)",
			configCapture: false, configSave: false, configCopy: false,
			args:        []string{"-notify-capture", "-notify-save=true", "-notify-copy", "version"},
			wantCapture: true, wantSave: true, wantCopy: true,
		},
		{
			name:          "flags override config (disable)",
			configCapture: true, configSave: true, configCopy: true,
			args:        []string{"-notify-capture=false", "-notify-save=false", "-notify-copy=false", "version"},
			wantCapture: false, wantSave: false, wantCopy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRoot()
			r.config.Notify.Capture = tt.configCapture
			r.config.Notify.Save = tt.configSave
			r.config.Notify.Copy = tt.configCopy

			// newRoot initializes the flags with the *current* config values.
			// Since we just changed the config values manually *after* newRoot,
			// we need to recreate the flags to simulate how newRoot does it normally.
			// Rebuilding flagset to reflect updated config
			r.fs = flag.NewFlagSet("shineyshot", flag.ContinueOnError)
			r.fs.BoolVar(&r.captureAlerts, "notify-capture", r.config.Notify.Capture, "show a desktop notification after capturing a screenshot")
			r.fs.BoolVar(&r.saveAlerts, "notify-save", r.config.Notify.Save, "show a desktop notification after saving an image")
			r.fs.BoolVar(&r.copyAlerts, "notify-copy", r.config.Notify.Copy, "show a desktop notification after copying to the clipboard")

			// Ignore errors in parse to just check flag values
			_ = r.fs.Parse(tt.args)

			if r.captureAlerts != tt.wantCapture {
				t.Errorf("Capture: got %v, want %v", r.captureAlerts, tt.wantCapture)
			}
			if r.saveAlerts != tt.wantSave {
				t.Errorf("Save: got %v, want %v", r.saveAlerts, tt.wantSave)
			}
			if r.copyAlerts != tt.wantCopy {
				t.Errorf("Copy: got %v, want %v", r.copyAlerts, tt.wantCopy)
			}
		})
	}
}
