//go:build !linux && !darwin && !windows

package platform

// Notify is a no-op on unsupported platforms.
func Notify(title, body string, opts Options) error {
	return nil
}
