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
	child := f.root.subcommand("file")
	defer func() {
		f.root.state = child.state
	}()
	switch f.op {
	case "capture":
		args := append([]string{"-output", f.path}, f.args...)
		cmd, err := parseSnapshotCmd(args, child)
		if err != nil {
			return err
		}
		return cmd.Run()
	case "draw":
		args := append([]string{"-file", f.path, "-output", f.path}, f.args...)
		cmd, err := parseDrawCmd(args, child)
		if err != nil {
			return err
		}
		return cmd.Run()
	case "annotate":
		flags := []string{}
		action := []string{}
		for i := 0; i < len(f.args); i++ {
			token := f.args[i]
			if strings.HasPrefix(token, "-") {
				flags = append(flags, token)
				if i+1 < len(f.args) && !strings.HasPrefix(f.args[i+1], "-") {
					flags = append(flags, f.args[i+1])
					i++
				}
				continue
			}
			action = append(action, f.args[i:]...)
			break
		}
		args := append([]string{"-file", f.path}, flags...)
		if len(action) == 0 {
			action = []string{"open"}
		}
		args = append(args, action...)
		cmd, err := parseAnnotateCmd(args, child)
		if err != nil {
			return err
		}
		return cmd.Run()
	case "preview":
		args := append([]string{"-file", f.path}, f.args...)
		cmd, err := parsePreviewCmd(args, child)
		if err != nil {
			return err
		}
		return cmd.Run()
	default:
		return &UsageError{of: f}
	}
}
