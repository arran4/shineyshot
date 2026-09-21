package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/arran4/shineyshot/internal/config"
)

func TestNotificationFlagsPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		args          []string
		wantCapture   bool
		wantSave      bool
		wantCopy      bool
	}{
		{
			name:          "no config file (defaults to false)",
			configContent: "",
			args:          []string{"version"},
			wantCapture:   false,
			wantSave:      false,
			wantCopy:      false,
		},
		{
			name: "config true",
			configContent: `[notify]
capture = true
save = true
copy = true`,
			args:        []string{"version"},
			wantCapture: true,
			wantSave:    true,
			wantCopy:    true,
		},
		{
			name: "config true but flag overrides to false",
			configContent: `[notify]
capture = true
save = true
copy = true`,
			args:        []string{"-notify-capture=false", "-notify-save=false", "-notify-copy=false", "version"},
			wantCapture: false,
			wantSave:    false,
			wantCopy:    false,
		},
		{
			name: "config false but flag overrides to true",
			configContent: `[notify]
capture = false
save = false
copy = false`,
			args:        []string{"-notify-capture=true", "-notify-save", "-notify-copy=true", "version"},
			wantCapture: true,
			wantSave:    true,
			wantCopy:    true,
		},
		{
			name: "mixed config",
			configContent: `[notify]
capture = true
save = false
copy = true`,
			args:        []string{"version"},
			wantCapture: true,
			wantSave:    false,
			wantCopy:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *config.Config
			if tt.configContent != "" {
				var err error
				cfg, err = config.Parse(strings.NewReader(tt.configContent))
				if err != nil {
					t.Fatalf("failed to parse config: %v", err)
				}
			} else {
				cfg = config.New()
			}

			r := newRootWithConfig(cfg)
			r.fs.Init("shineyshot", flag.ContinueOnError)

			if err := r.fs.Parse(tt.args); err != nil {
				t.Fatalf("fs.Parse failed: %v", err)
			}

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
