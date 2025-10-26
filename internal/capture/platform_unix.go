//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

type x11Backend struct{}

func newBackend() platformBackend {
	return x11Backend{}
}

func runningOnWayland() bool {
	sessionType := strings.ToLower(strings.TrimSpace(os.Getenv("XDG_SESSION_TYPE")))
	if sessionType == "wayland" {
		return true
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return false
}

func (x11Backend) ListMonitors() ([]MonitorInfo, error) {
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

func (x11Backend) ListWindows() ([]WindowInfo, error) {
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

func (x11Backend) CaptureWindowImage(id uint32) (*image.RGBA, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect X server: %w", err)
	}
	defer conn.Close()

	geom, err := xproto.GetGeometry(conn, xproto.Drawable(id)).Reply()
	if err != nil {
		return nil, fmt.Errorf("window geometry: %w", err)
	}
	width := int(geom.Width)
	height := int(geom.Height)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("window has empty geometry")
	}

	setup := xproto.Setup(conn)
	if setup == nil {
		return nil, fmt.Errorf("xproto setup unavailable")
	}

	reply, err := xproto.GetImage(conn, xproto.ImageFormatZPixmap, xproto.Drawable(id), 0, 0, geom.Width, geom.Height, ^uint32(0)).Reply()
	if err != nil {
		return nil, fmt.Errorf("window pixels: %w", err)
	}

	img, err := xImageToRGBA(setup, reply, width, height, "window")
	if err != nil {
		return nil, err
	}
	return img, nil
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
