package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/config"
	"github.com/example/shineyshot/internal/notify"
	"github.com/example/shineyshot/internal/theme"
)

var (
	version            = "dev"
	commit             = ""
	date               = ""
	configPathOverride = ""
)

type runnable interface{ Run() error }

type root struct {
	fs            *flag.FlagSet
	program       string
	state         *appstate.AppState
	notifier      *notify.Notifier
	config        *config.Config
	captureAlerts bool
	saveAlerts    bool
	copyAlerts    bool
	themeName     string
	activeTheme   *theme.Theme
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
		config:        r.config,
		captureAlerts: r.captureAlerts,
		saveAlerts:    r.saveAlerts,
		copyAlerts:    r.copyAlerts,
		themeName:     r.themeName,
		activeTheme:   r.activeTheme,
	}
}

func (r *root) FlagSet() *flag.FlagSet {
	return r.fs
}

func newRoot() *root {
	prefs := notify.LoadPreferences()
	loader := config.NewLoader(version, configPathOverride)
	cfg, err := loader.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load config: %v\n", err)
		cfg = config.New()
	}

	r := &root{
		fs:       flag.NewFlagSet("shineyshot", flag.ExitOnError),
		program:  "shineyshot",
		notifier: notify.New(prefs),
		config:   cfg,
	}
	r.fs.BoolVar(&r.captureAlerts, "notify-capture", cfg.Notify.Capture, "show a desktop notification after capturing a screenshot")
	r.fs.BoolVar(&r.saveAlerts, "notify-save", cfg.Notify.Save, "show a desktop notification after saving an image")
	r.fs.BoolVar(&r.copyAlerts, "notify-copy", cfg.Notify.Copy, "show a desktop notification after copying to the clipboard")

	// Precedence: CLI > Env > Config > Default
	// We set the default value for the flag to "", and handle fallback logic in Run if it remains empty.
	r.fs.StringVar(&r.themeName, "theme", "", "color theme to use (default, dark, high_contrast, hotdog)")
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

	// Load theme if specified via CLI, Env, or Config
	themeName := r.themeName
	if themeName == "" {
		themeName = os.Getenv("SHINEYSHOT_THEME")
	}
	if themeName == "" {
		themeName = r.config.Theme
	}

	var t *theme.Theme
	// 1. Check loaded themes from Config
	if cfgTheme, ok := r.config.Themes[themeName]; ok {
		t = cfgTheme
	} else {
		// 2. Fallback to standard theme loader (File / Embedded / System)
		loader := theme.NewLoader()
		var loadErr error
		t, loadErr = loader.Load(themeName)
		if loadErr != nil {
			// Only warn if a specific theme was requested but failed to load.
			// If themeName is empty (implicit default), Loader returns Default silently if passed "",
			// but here we might have "default" explicitly or implicitly.
			// Loader.Load("") returns Default.
			if themeName != "" && themeName != "default" {
				fmt.Fprintf(os.Stderr, "warning: failed to load theme '%s': %v. using default.\n", themeName, loadErr)
			}
			t = theme.Default()
		}
	}

	// Inject theme into AppState options when creating state in subcommands
	// Note: We need to modify how subcommands create state or pass it down.
	// Currently subcommands create their own state or modify `r.state`.
	// `r.state` seems to be nil initially.
	// Most commands call `appstate.New(...)`. We need to ensure they use the loaded theme.
	// We can store the theme in `root` and have subcommands use it.
	r.activeTheme = t

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
	case "config":
		cmd, err = parseConfigCmd(subArgs, r)
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
