//go:build linux

package platform

import (
	"github.com/godbus/dbus/v5"
)

// Notify sends a desktop notification using the Freedesktop.org notification spec.
func Notify(title, body string, opts Options) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	call := obj.Call("org.freedesktop.Notifications.Notify", 0,
		"ShineyShot", uint32(0), opts.IconPath, title, body, []string{}, map[string]dbus.Variant{}, int32(5000))
	return call.Err
}
