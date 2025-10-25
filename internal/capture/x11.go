package capture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

// MonitorInfo describes an individual monitor in the X11 layout.
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

var (
	errNoMonitors = errors.New("no monitors available")
	errNoWindows  = errors.New("no windows available")
)

// ListMonitors retrieves all monitors using the X RandR extension.
func ListMonitors() ([]MonitorInfo, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect X server: %w", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	if setup == nil {
		return nil, fmt.Errorf("xproto setup unavailable")
	}
	screen := setup.DefaultScreen(conn)
	if screen == nil {
		return nil, fmt.Errorf("xproto screen unavailable")
	}

	if err := randr.Init(conn); err != nil {
		return nil, fmt.Errorf("init randr: %w", err)
	}

	monitors, err := fetchMonitors(conn, screen.Root)
	if err != nil {
		return nil, err
	}
	if len(monitors) == 0 {
		return nil, errNoMonitors
	}
	return monitors, nil
}

// ListWindows retrieves the available top-level windows in stacking order.
func ListWindows() ([]WindowInfo, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect X server: %w", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	if setup == nil {
		return nil, fmt.Errorf("xproto setup unavailable")
	}
	screen := setup.DefaultScreen(conn)
	if screen == nil {
		return nil, fmt.Errorf("xproto screen unavailable")
	}

	monitors, _ := fetchMonitors(conn, screen.Root)
	activeID, _ := fetchActiveWindow(conn, screen.Root)

	windows, err := fetchWindows(conn, screen.Root, monitors, activeID)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, errNoWindows
	}
	return windows, nil
}

func fetchMonitors(conn *xgb.Conn, root xproto.Window) ([]MonitorInfo, error) {
	if err := randr.Init(conn); err != nil {
		return nil, fmt.Errorf("init randr: %w", err)
	}
	res, err := randr.GetScreenResources(conn, root).Reply()
	if err != nil {
		return nil, fmt.Errorf("randr screen resources: %w", err)
	}
	primaryOutput := randr.Output(0)
	if primary, err := randr.GetOutputPrimary(conn, root).Reply(); err == nil {
		primaryOutput = primary.Output
	}
	monitors := make([]MonitorInfo, 0, len(res.Outputs))
	idx := 0
	for _, output := range res.Outputs {
		info, err := randr.GetOutputInfo(conn, output, res.ConfigTimestamp).Reply()
		if err != nil {
			continue
		}
		if info.Connection != randr.ConnectionConnected || info.Crtc == 0 {
			continue
		}
		crtc, err := randr.GetCrtcInfo(conn, info.Crtc, res.ConfigTimestamp).Reply()
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(info.Name))
		rect := image.Rect(
			int(crtc.X),
			int(crtc.Y),
			int(crtc.X)+int(crtc.Width),
			int(crtc.Y)+int(crtc.Height),
		)
		monitors = append(monitors, MonitorInfo{
			Index:   idx,
			Name:    name,
			Rect:    rect,
			Primary: output == primaryOutput,
		})
		idx++
	}
	return monitors, nil
}

func fetchActiveWindow(conn *xgb.Conn, root xproto.Window) (uint32, error) {
	atom, err := internAtom(conn, "_NET_ACTIVE_WINDOW")
	if err != nil {
		return 0, err
	}
	reply, err := xproto.GetProperty(conn, false, root, atom, xproto.AtomWindow, 0, 1).Reply()
	if err != nil {
		return 0, err
	}
	if reply.Format != 32 || reply.ValueLen == 0 {
		return 0, fmt.Errorf("active window unavailable")
	}
	return xgb.Get32(reply.Value), nil
}

func fetchWindows(conn *xgb.Conn, root xproto.Window, monitors []MonitorInfo, activeID uint32) ([]WindowInfo, error) {
	listAtom, err := internAtom(conn, "_NET_CLIENT_LIST_STACKING")
	if err != nil {
		return nil, err
	}
	reply, err := xproto.GetProperty(conn, false, root, listAtom, xproto.AtomWindow, 0, 1<<16).Reply()
	if err != nil || reply.Format != 32 || reply.ValueLen == 0 {
		// Fallback to _NET_CLIENT_LIST if stacking not available.
		listAtom, err = internAtom(conn, "_NET_CLIENT_LIST")
		if err != nil {
			return nil, err
		}
		reply, err = xproto.GetProperty(conn, false, root, listAtom, xproto.AtomWindow, 0, 1<<16).Reply()
		if err != nil {
			return nil, err
		}
	}
	ids := make([]xproto.Window, 0, reply.ValueLen)
	for idx := 0; idx < int(reply.ValueLen); idx++ {
		wid := xgb.Get32(reply.Value[idx*4:])
		ids = append(ids, xproto.Window(wid))
	}
	if len(ids) == 0 {
		return nil, nil
	}

	windows := make([]WindowInfo, 0, len(ids))
	for idx := len(ids) - 1; idx >= 0; idx-- {
		win := ids[idx]
		info, err := describeWindow(conn, root, win)
		if err != nil {
			continue
		}
		info.Index = len(windows)
		info.Active = info.ID == activeID
		info.Monitor = monitorForRect(info.Rect, monitors)
		windows = append(windows, info)
	}
	return windows, nil
}

func describeWindow(conn *xgb.Conn, root xproto.Window, win xproto.Window) (WindowInfo, error) {
	title := readUTF8Property(conn, win, "_NET_WM_NAME")
	if title == "" {
		title = readStringProperty(conn, win, "WM_NAME")
	}
	class, instance := readClass(conn, win)
	pid := readPID(conn, win)
	exec := readExecutable(pid)
	rect, err := windowRect(conn, root, win)
	if err != nil {
		return WindowInfo{}, err
	}
	return WindowInfo{
		ID:         uint32(win),
		Title:      title,
		Class:      class,
		Instance:   instance,
		PID:        pid,
		Executable: exec,
		Rect:       rect,
		Monitor:    -1,
	}, nil
}

func windowRect(conn *xgb.Conn, root xproto.Window, win xproto.Window) (image.Rectangle, error) {
	geo, err := xproto.GetGeometry(conn, xproto.Drawable(win)).Reply()
	if err != nil {
		return image.Rectangle{}, err
	}
	trans, err := xproto.TranslateCoordinates(conn, win, root, int16(geo.X), int16(geo.Y)).Reply()
	if err != nil {
		return image.Rectangle{}, err
	}
	x := int(trans.DstX) - int(geo.BorderWidth)
	y := int(trans.DstY) - int(geo.BorderWidth)
	width := int(geo.Width) + int(geo.BorderWidth)*2
	height := int(geo.Height) + int(geo.BorderWidth)*2
	return image.Rect(x, y, x+width, y+height), nil
}

func monitorForRect(rect image.Rectangle, monitors []MonitorInfo) int {
	if len(monitors) == 0 {
		return -1
	}
	center := image.Point{X: rect.Min.X + rect.Dx()/2, Y: rect.Min.Y + rect.Dy()/2}
	best := -1
	for _, mon := range monitors {
		if center.In(mon.Rect) {
			return mon.Index
		}
		if best == -1 {
			best = mon.Index
		}
	}
	return best
}

func internAtom(conn *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(conn, true, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, err
	}
	return reply.Atom, nil
}

func readUTF8Property(conn *xgb.Conn, win xproto.Window, name string) string {
	atom, err := internAtom(conn, name)
	if err != nil {
		return ""
	}
	utf8StringAtom, err := internAtom(conn, "UTF8_STRING")
	if err != nil {
		return ""
	}
	reply, err := xproto.GetProperty(conn, false, win, atom, utf8StringAtom, 0, 1<<16).Reply()
	if err != nil || reply.ValueLen == 0 {
		return ""
	}
	return strings.TrimRight(string(reply.Value), "\x00")
}

func readStringProperty(conn *xgb.Conn, win xproto.Window, name string) string {
	atom, err := internAtom(conn, name)
	if err != nil {
		return ""
	}
	reply, err := xproto.GetProperty(conn, false, win, atom, xproto.AtomString, 0, 1<<16).Reply()
	if err != nil || reply.ValueLen == 0 {
		return ""
	}
	return strings.TrimRight(string(reply.Value), "\x00")
}

func readClass(conn *xgb.Conn, win xproto.Window) (class string, instance string) {
	atom, err := internAtom(conn, "WM_CLASS")
	if err != nil {
		return "", ""
	}
	reply, err := xproto.GetProperty(conn, false, win, atom, xproto.AtomString, 0, 64).Reply()
	if err != nil || reply.ValueLen == 0 {
		return "", ""
	}
	parts := bytes.Split(reply.Value, []byte{0})
	vals := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		vals = append(vals, string(p))
	}
	if len(vals) >= 2 {
		return vals[1], vals[0]
	}
	if len(vals) == 1 {
		return vals[0], vals[0]
	}
	return "", ""
}

func readPID(conn *xgb.Conn, win xproto.Window) uint32 {
	atom, err := internAtom(conn, "_NET_WM_PID")
	if err != nil {
		return 0
	}
	reply, err := xproto.GetProperty(conn, false, win, atom, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil || reply.Format != 32 || reply.ValueLen == 0 {
		return 0
	}
	return xgb.Get32(reply.Value)
}

func readExecutable(pid uint32) string {
	if pid == 0 {
		return ""
	}
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		return strings.TrimSpace(string(data))
	}
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return filepath.Base(exe)
	}
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		parts := bytes.Split(data, []byte{0})
		if len(parts) > 0 && len(parts[0]) > 0 {
			return filepath.Base(string(parts[0]))
		}
	}
	return ""
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
