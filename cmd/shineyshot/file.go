package main

import (
	"flag"
	"strings"
)

type fileCmd struct {
	path string
	op   string
	args []string
	*root
	fs *flag.FlagSet
}

func (f *fileCmd) FlagSet() *flag.FlagSet {
	return f.fs
}

func (f *fileCmd) Template() string {
	return "file.txt"
}

func parseFileCmd(args []string, r *root) (*fileCmd, error) {
	fs := flag.NewFlagSet("file", flag.ExitOnError)
	cmd := &fileCmd{root: r, fs: fs}
	fs.Usage = usageFunc(cmd)
	fs.StringVar(&cmd.path, "file", "", "path to the image file to read or write")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if cmd.path == "" || fs.NArg() < 1 {
		return nil, &UsageError{of: cmd}
	}
	cmd.op = strings.ToLower(fs.Arg(0))
	cmd.args = fs.Args()[1:]
	return cmd, nil
}

func (f *fileCmd) Run() error {
	switch f.op {
	case "snapshot":
		args := append([]string{"-output", f.path}, f.args...)
		cmd, err := parseSnapshotCmd(args, f.root)
		if err != nil {
			return err
		}
		return cmd.Run()
	case "draw":
		args := append([]string{"-file", f.path, "-output", f.path}, f.args...)
		cmd, err := parseDrawCmd(args, f.root)
		if err != nil {
			return err
		}
		return cmd.Run()
	case "annotate":
		args := []string{"open-file", "-file", f.path, "-output", f.path}
		args = append(args, f.args...)
		cmd, err := parseAnnotateCmd(args, f.root)
		if err != nil {
			return err
		}
		return cmd.Run()
	case "preview":
		args := append([]string{"-file", f.path}, f.args...)
		cmd, err := parsePreviewCmd(args, f.root)
		if err != nil {
			return err
		}
		return cmd.Run()
	default:
		return &UsageError{of: f}
	}
}
