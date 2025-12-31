package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/notify"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

type runnable interface{ Run() error }

type root struct {
	fs            *flag.FlagSet
	program       string
	state         *appstate.AppState
	notifier      *notify.Notifier
	captureAlerts bool
	saveAlerts    bool
	copyAlerts    bool
}

func (r *root) Program() string {
	return r.program
}

func (r *root) subcommand(name string) *root {
	program := strings.TrimSpace(strings.Join([]string{r.program, name}, " "))
	return &root{
		program:       program,
		state:         r.state,
		notifier:      r.notifier,
		captureAlerts: r.captureAlerts,
		saveAlerts:    r.saveAlerts,
		copyAlerts:    r.copyAlerts,
	}
}

func (r *root) FlagSet() *flag.FlagSet {
	return r.fs
}

func newRoot() *root {
	prefs := notify.LoadPreferences()
	r := &root{
		fs:       flag.NewFlagSet("shineyshot", flag.ExitOnError),
		program:  "shineyshot",
		notifier: notify.New(prefs),
	}
	r.fs.BoolVar(&r.captureAlerts, "notify-capture", false, "show a desktop notification after capturing a screenshot")
	r.fs.BoolVar(&r.saveAlerts, "notify-save", false, "show a desktop notification after saving an image")
	r.fs.BoolVar(&r.copyAlerts, "notify-copy", false, "show a desktop notification after copying to the clipboard")
	r.fs.Usage = usageFunc(r)
	return r
}

func (r *root) Run(args []string) error {
	if err := r.fs.Parse(args); err != nil {
		return err
	}
	if r.fs.NArg() < 1 {
		return &UsageError{of: r}
	}
	if r.notifier != nil {
		r.notifier.Enable(notify.EventCapture, r.captureAlerts)
		r.notifier.Enable(notify.EventSave, r.saveAlerts)
		r.notifier.Enable(notify.EventCopy, r.copyAlerts)
	}
	cmdName := r.fs.Arg(0)
	subArgs := r.fs.Args()[1:]

	var (
		cmd runnable
		err error
	)
	switch cmdName {
	case "annotate":
		cmd, err = parseAnnotateCmd(subArgs, r)
	case "preview":
		cmd, err = parsePreviewCmd(subArgs, r)
	case "snapshot":
		cmd, err = parseSnapshotCmd(subArgs, r)
	case "draw":
		cmd, err = parseDrawCmd(subArgs, r)
	case "file":
		cmd, err = parseFileCmd(subArgs, r)
	case "interactive":
		cmd, err = parseInteractiveCmd(subArgs, r)
	case "background":
		cmd, err = parseBackgroundCmd(subArgs, r)
	case "windows":
		cmd, err = parseWindowsCmd(subArgs, r)
	case "colors":
		cmd, err = parseColorsCmd(subArgs, r)
	case "widths":
		cmd, err = parseWidthsCmd(subArgs, r)
	case "test":
		cmd, err = parseTestCmd(subArgs, r)
	case "version":
		cmd = &versionCmd{r: r}
	default:
		err = &UsageError{of: r}
	}
	if err != nil {
		return err
	}
	if runErr := cmd.Run(); runErr != nil {
		return runErr
	}
	return nil
}

func main() {
	r := newRoot()
	if err := r.Run(os.Args[1:]); err != nil {
		var uerr *UsageError
		if errors.As(err, &uerr) {
			fmt.Fprintln(os.Stderr, uerr.Error())
		} else {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func (r *root) notifyCapture(detail string, img image.Image) {
	if r == nil || r.notifier == nil {
		return
	}
	r.notifier.Capture(detail, img)
}

func (r *root) notifySave(path string) {
	if r == nil || r.notifier == nil {
		return
	}
	r.notifier.Save(path)
}

func (r *root) notifyCopy(detail string) {
	if r == nil || r.notifier == nil {
		return
	}
	r.notifier.Copy(detail)
}
