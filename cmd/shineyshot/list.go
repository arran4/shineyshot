package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
)

type windowsCmd struct {
	*root
	fs *flag.FlagSet
}

func parseWindowsCmd(args []string, r *root) (*windowsCmd, error) {
	fs := flag.NewFlagSet("windows", flag.ExitOnError)
	cmd := &windowsCmd{root: r, fs: fs}
	fs.Usage = usageFunc(cmd)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, &UsageError{of: cmd}
	}
	return cmd, nil
}

func (c *windowsCmd) Run() error {
	windows, err := capture.ListWindows()
	if err != nil {
		return err
	}
	if len(windows) == 0 {
		fmt.Fprintln(os.Stdout, "no windows available")
		return nil
	}
	fmt.Fprintln(os.Stdout, "available windows (* marks the active window):")
	for _, win := range windows {
		marker := " "
		if win.Active {
			marker = "*"
		}
		fmt.Fprintf(os.Stdout, "%s %s\n", marker, formatWindowLabel(win))
	}
	fmt.Fprintln(os.Stdout, "selectors: index:<n>, id:<hex>, pid:<pid>, exec:<name>, class:<name>, title:<text>, substring match")
	return nil
}

func (c *windowsCmd) FlagSet() *flag.FlagSet {
	return c.fs
}

func (c *windowsCmd) Template() string {
	return "windows.txt"
}

type colorsCmd struct {
	*root
	fs *flag.FlagSet
}

func parseColorsCmd(args []string, r *root) (*colorsCmd, error) {
	fs := flag.NewFlagSet("colors", flag.ExitOnError)
	cmd := &colorsCmd{root: r, fs: fs}
	fs.Usage = usageFunc(cmd)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, &UsageError{of: cmd}
	}
	return cmd, nil
}

func (c *colorsCmd) Run() error {
	palette := appstate.PaletteColors()
	if len(palette) == 0 {
		fmt.Fprintln(os.Stdout, "no colors available")
		return nil
	}
	fmt.Fprintln(os.Stdout, "available palette colors (* marks the default color):")
	defaultIdx := clampIndex(appstate.DefaultColorIndex(), len(palette))
	for idx, entry := range palette {
		marker := " "
		if idx == defaultIdx {
			marker = "*"
		}
		name := entry.Name
		hex := fmt.Sprintf("#%02X%02X%02X", entry.Color.R, entry.Color.G, entry.Color.B)
		if name == "" {
			name = hex
		}
		block := fmt.Sprintf("\x1b[48;2;%d;%d;%dm  \x1b[0m", entry.Color.R, entry.Color.G, entry.Color.B)
		fmt.Fprintf(os.Stdout, "%s %2d: %-12s %s %s\n", marker, idx, name, hex, block)
	}
	return nil
}

func (c *colorsCmd) FlagSet() *flag.FlagSet {
	return c.fs
}

func (c *colorsCmd) Template() string {
	return "colors.txt"
}

type widthsCmd struct {
	*root
	fs *flag.FlagSet
}

func parseWidthsCmd(args []string, r *root) (*widthsCmd, error) {
	fs := flag.NewFlagSet("widths", flag.ExitOnError)
	cmd := &widthsCmd{root: r, fs: fs}
	fs.Usage = usageFunc(cmd)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() != 0 {
		return nil, &UsageError{of: cmd}
	}
	return cmd, nil
}

func (c *widthsCmd) Run() error {
	widths := appstate.WidthOptions()
	if len(widths) == 0 {
		fmt.Fprintln(os.Stdout, "no widths available")
		return nil
	}
	fmt.Fprintln(os.Stdout, "available stroke widths (* marks the default width):")
	defaultIdx := clampIndex(appstate.DefaultWidthIndex(), len(widths))
	for idx, width := range widths {
		marker := " "
		if idx == defaultIdx {
			marker = "*"
		}
		fmt.Fprintf(os.Stdout, "%s %3dpx\n", marker, width)
	}
	return nil
}

func (c *widthsCmd) FlagSet() *flag.FlagSet {
	return c.fs
}

func (c *widthsCmd) Template() string {
	return "widths.txt"
}
