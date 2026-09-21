package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestNotificationFlagsPrecedence(t *testing.T) {
	// Create a temporary directory for config files
	tmpDir := t.TempDir()

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
			var configPath string
			if tt.configContent != "" {
				configPath = filepath.Join(tmpDir, "config.rc")
				err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
				if err != nil {
					t.Fatalf("failed to write temp config: %v", err)
				}
			} else {
				// Point to a non-existent file to simulate no config
				configPath = filepath.Join(tmpDir, "non_existent.rc")
			}

			// Override the package-level variable to point to our test config
			configPathOverride = configPath
			defer func() { configPathOverride = "" }()

			r := newRoot()
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
