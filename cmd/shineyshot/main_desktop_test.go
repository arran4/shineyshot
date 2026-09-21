package main

import (
	"flag"
	"os"
	"strings"
	"testing"
)

func TestDesktopExecCommands(t *testing.T) {
	content, err := os.ReadFile("../../assets/shineyshot.desktop")
	if err != nil {
		t.Fatalf("Failed to read desktop file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Exec=") {
			cmdStr := strings.TrimSpace(strings.TrimPrefix(line, "Exec="))
			parts := strings.Split(cmdStr, " ")
			if len(parts) < 1 || parts[0] != "shineyshot" {
				t.Errorf("Invalid Exec command: %s", cmdStr)
				continue
			}

			args := parts[1:]
			if len(args) == 0 {
				t.Errorf("Exec command misses sub-command: %s", cmdStr)
				continue
			}
			r := newRoot()

			// Replace standard os.Exit with panic in flag parsing
			r.fs.Init("shineyshot", flag.ContinueOnError)

			cmdName := args[0]
			subArgs := args[1:]
			var cmd runnable
			var parseErr error

			switch cmdName {
			case "annotate":
				cmd, parseErr = parseAnnotateCmd(subArgs, r)
			case "interactive":
				cmd, parseErr = parseInteractiveCmd(subArgs, r)
			case "editor":
				cmd, parseErr = parseEditorCmd(subArgs, r)
			default:
				t.Errorf("Unexpected command in desktop file: %s", cmdName)
				continue
			}

			if parseErr != nil {
				t.Errorf("Failed to parse Exec command '%s': %v", cmdStr, parseErr)
			} else if cmd == nil {
				t.Errorf("Parsed command was nil for '%s'", cmdStr)
			}
		}
	}
}
