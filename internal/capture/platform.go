package capture

import (
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"
)

type platformBackend interface {
	ListMonitors() ([]MonitorInfo, error)
	ListWindows() ([]WindowInfo, error)
	CaptureWindowImage(uint32) (*image.RGBA, error)
}

var backend = newBackend()

var (
	errNoMonitors = errors.New("no monitors available")
	errNoWindows  = errors.New("no windows available")
)

// MonitorInfo describes an individual monitor in the display layout.
type MonitorInfo struct {
	Index   int
	Name    string
	Rect    image.Rectangle
	Primary bool
}

// WindowInfo describes a top-level window available for capture.
type WindowInfo struct {
	Index      int
	ID         uint32
	Title      string
	Class      string
	Instance   string
	PID        uint32
	Executable string
	Rect       image.Rectangle
	Monitor    int
	Active     bool
}

// ListMonitors retrieves all monitors using the platform backend.
func ListMonitors() ([]MonitorInfo, error) {
	return backend.ListMonitors()
}

// ListWindows retrieves the available top-level windows using the platform backend.
func ListWindows() ([]WindowInfo, error) {
	return backend.ListWindows()
}

func captureWindowImage(id uint32) (*image.RGBA, error) {
	return backend.CaptureWindowImage(id)
}

// FindMonitor resolves a monitor selector against the provided list.
func FindMonitor(monitors []MonitorInfo, selector string) (MonitorInfo, error) {
	if len(monitors) == 0 {
		return MonitorInfo{}, errNoMonitors
	}
	if selector == "" {
		return monitors[0], nil
	}
	sel := strings.TrimSpace(selector)
	lower := strings.ToLower(sel)
	if lower == "primary" {
		for _, mon := range monitors {
			if mon.Primary {
				return mon, nil
			}
		}
		return monitors[0], nil
	}
	if strings.HasPrefix(lower, "#") {
		lower = lower[1:]
	}
	if idx, err := strconv.Atoi(lower); err == nil {
		if idx < 0 || idx >= len(monitors) {
			return MonitorInfo{}, fmt.Errorf("monitor index %d out of range", idx)
		}
		return monitors[idx], nil
	}
	for _, mon := range monitors {
		if strings.Contains(strings.ToLower(mon.Name), lower) {
			return mon, nil
		}
	}
	return MonitorInfo{}, fmt.Errorf("monitor %q not found", selector)
}

// SelectWindow matches a selector string against the list of windows.
func SelectWindow(selector string, windows []WindowInfo) (WindowInfo, error) {
	if len(windows) == 0 {
		return WindowInfo{}, errNoWindows
	}
	sel := strings.TrimSpace(selector)
	if sel == "" {
		for _, win := range windows {
			if win.Active {
				return win, nil
			}
		}
		return windows[len(windows)-1], nil
	}
	lower := strings.ToLower(sel)
	if lower == "active" {
		for _, win := range windows {
			if win.Active {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("no active window detected")
	}
	if strings.HasPrefix(lower, "index:") {
		val := strings.TrimSpace(lower[6:])
		idx, err := strconv.Atoi(val)
		if err != nil {
			return WindowInfo{}, fmt.Errorf("invalid index %q", val)
		}
		if idx < 0 || idx >= len(windows) {
			return WindowInfo{}, fmt.Errorf("window index %d out of range", idx)
		}
		return windows[idx], nil
	}
	if strings.HasPrefix(lower, "id:") {
		val := strings.TrimSpace(lower[3:])
		id, err := parseWindowID(val)
		if err != nil {
			return WindowInfo{}, err
		}
		for _, win := range windows {
			if win.ID == id {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("window id 0x%x not found", id)
	}
	if strings.HasPrefix(lower, "pid:") {
		val := strings.TrimSpace(lower[4:])
		pid64, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return WindowInfo{}, fmt.Errorf("invalid pid %q", val)
		}
		pid := uint32(pid64)
		for _, win := range windows {
			if win.PID == pid {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("window with pid %d not found", pid)
	}
	if strings.HasPrefix(lower, "exec:") {
		needle := strings.TrimSpace(lower[5:])
		for _, win := range windows {
			if strings.Contains(strings.ToLower(win.Executable), needle) {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("window with exec %q not found", needle)
	}
	if strings.HasPrefix(lower, "class:") {
		needle := strings.TrimSpace(lower[6:])
		for _, win := range windows {
			if strings.Contains(strings.ToLower(win.Class), needle) || strings.Contains(strings.ToLower(win.Instance), needle) {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("window with class %q not found", needle)
	}
	if strings.HasPrefix(lower, "title:") {
		needle := strings.TrimSpace(sel[6:])
		lowerNeedle := strings.ToLower(needle)
		for _, win := range windows {
			if strings.Contains(strings.ToLower(win.Title), lowerNeedle) {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("window with title %q not found", needle)
	}
	if strings.HasPrefix(lower, "name:") {
		needle := strings.TrimSpace(sel[5:])
		lowerNeedle := strings.ToLower(needle)
		for _, win := range windows {
			if strings.Contains(strings.ToLower(win.Title), lowerNeedle) {
				return win, nil
			}
		}
		return WindowInfo{}, fmt.Errorf("window with title %q not found", needle)
	}
	if idx, err := strconv.Atoi(sel); err == nil {
		if idx < 0 || idx >= len(windows) {
			return WindowInfo{}, fmt.Errorf("window index %d out of range", idx)
		}
		return windows[idx], nil
	}
	if strings.HasPrefix(lower, "0x") {
		id, err := parseWindowID(sel)
		if err == nil {
			for _, win := range windows {
				if win.ID == id {
					return win, nil
				}
			}
			return WindowInfo{}, fmt.Errorf("window id 0x%x not found", id)
		}
	}
	needle := strings.ToLower(sel)
	for _, win := range windows {
		if strings.Contains(strings.ToLower(win.Title), needle) {
			return win, nil
		}
		if strings.Contains(strings.ToLower(win.Executable), needle) {
			return win, nil
		}
		if strings.Contains(strings.ToLower(win.Class), needle) {
			return win, nil
		}
		if strings.Contains(strings.ToLower(win.Instance), needle) {
			return win, nil
		}
	}
	return WindowInfo{}, fmt.Errorf("no window matched %q", selector)
}

func parseWindowID(val string) (uint32, error) {
	v := strings.TrimSpace(val)
	if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
		parsed, err := strconv.ParseUint(v[2:], 16, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid window id %q", val)
		}
		return uint32(parsed), nil
	}
	parsed, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid window id %q", val)
	}
	return uint32(parsed), nil
}
