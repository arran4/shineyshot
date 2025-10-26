//go:build darwin

package platform

import (
	"fmt"
	"os/exec"
)

// Notify displays a desktop notification using macOS Notification Center.
func Notify(title, body string, opts Options) error {
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}
