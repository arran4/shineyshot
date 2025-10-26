package notify

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/example/shineyshot/internal/platform"
)

// Event identifies a notification trigger.
type Event string

const (
	// EventCapture emits a notification when a capture completes.
	EventCapture Event = "capture"
	// EventSave emits a notification when an image is persisted to disk.
	EventSave Event = "save"
	// EventCopy emits a notification when data is copied to the clipboard.
	EventCopy Event = "copy"
)

// EventPreference describes formatting for a notification event.
type EventPreference struct {
	Template string
}

// Preferences describes notification behaviour loaded from configuration.
type Preferences struct {
	Title  string
	Events map[Event]EventPreference
}

// DefaultPreferences returns the default notification settings.
func DefaultPreferences() Preferences {
	return Preferences{
		Title: "ShineyShot",
		Events: map[Event]EventPreference{
			EventCapture: {Template: "Captured %s"},
			EventSave:    {Template: "Saved %s"},
			EventCopy:    {Template: "Copied %s to clipboard"},
		},
	}
}

// LoadPreferences reads configuration from environment variables.
func LoadPreferences() Preferences {
	prefs := DefaultPreferences()
	if v := strings.TrimSpace(os.Getenv("SHINEYSHOT_NOTIFY_TITLE")); v != "" {
		prefs.Title = v
	}
	apply := func(key string, event Event) {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			eventPrefs := prefs.Events[event]
			eventPrefs.Template = v
			prefs.Events[event] = eventPrefs
		}
	}
	apply("SHINEYSHOT_NOTIFY_CAPTURE_TEXT", EventCapture)
	apply("SHINEYSHOT_NOTIFY_SAVE_TEXT", EventSave)
	apply("SHINEYSHOT_NOTIFY_COPY_TEXT", EventCopy)
	return prefs
}

// Notifier sends OS-level notifications based on the configured preferences.
type Notifier struct {
	prefs   Preferences
	enabled map[Event]bool
}

// New creates a new Notifier using the provided preferences.
func New(prefs Preferences) *Notifier {
	cloned := Preferences{Title: prefs.Title, Events: make(map[Event]EventPreference, len(prefs.Events))}
	for k, v := range prefs.Events {
		cloned.Events[k] = v
	}
	return &Notifier{prefs: cloned, enabled: make(map[Event]bool)}
}

// Enable toggles the notifier for the provided event.
func (n *Notifier) Enable(event Event, enabled bool) {
	if n == nil {
		return
	}
	if n.enabled == nil {
		n.enabled = make(map[Event]bool)
	}
	n.enabled[event] = enabled
}

// Capture sends a capture notification with an optional image preview.
func (n *Notifier) Capture(detail string, img image.Image) {
	if !n.enabledFor(EventCapture) {
		return
	}
	opts := platform.Options{}
	if img != nil {
		if path, cleanup, err := createPreview(img); err != nil {
			log.Printf("notification preview: %v", err)
		} else {
			defer cleanup()
			opts.IconPath = path
		}
	}
	n.dispatch(EventCapture, detail, opts)
}

// Save sends a save notification including the written filename when available.
func (n *Notifier) Save(path string) {
	if !n.enabledFor(EventSave) {
		return
	}
	detail := strings.TrimSpace(path)
	opts := platform.Options{}
	if abs, err := filepath.Abs(path); err == nil {
		detail = abs
		if _, statErr := os.Stat(abs); statErr == nil {
			opts.IconPath = abs
		}
	}
	n.dispatch(EventSave, detail, opts)
}

// Copy sends a clipboard notification.
func (n *Notifier) Copy(detail string) {
	if !n.enabledFor(EventCopy) {
		return
	}
	if strings.TrimSpace(detail) == "" {
		detail = "image"
	}
	n.dispatch(EventCopy, detail, platform.Options{})
}

func (n *Notifier) enabledFor(event Event) bool {
	if n == nil {
		return false
	}
	if n.enabled == nil {
		return false
	}
	return n.enabled[event]
}

func (n *Notifier) dispatch(event Event, detail string, opts platform.Options) {
	if !n.enabledFor(event) {
		return
	}
	template := strings.TrimSpace(n.template(event))
	if template == "" {
		return
	}
	body := strings.TrimSpace(fmt.Sprintf(template, strings.TrimSpace(detail)))
	if body == "" {
		return
	}
	if err := platform.Notify(n.prefs.Title, body, opts); err != nil {
		log.Printf("notification %s: %v", event, err)
	}
}

func (n *Notifier) template(event Event) string {
	if n == nil {
		return ""
	}
	if pref, ok := n.prefs.Events[event]; ok {
		return pref.Template
	}
	return ""
}

func createPreview(img image.Image) (string, func(), error) {
	f, err := os.CreateTemp("", "shineyshot-preview-*.png")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, err
	}
	cleanup := func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("remove preview: %v", err)
		}
	}
	return path, cleanup, nil
}
