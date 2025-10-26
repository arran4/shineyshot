package platform

// Options configures how a notification is displayed on the host platform.
type Options struct {
	// IconPath, when non-empty, points to an image file the notification center
	// should display with the notification if supported by the platform.
	IconPath string
}
