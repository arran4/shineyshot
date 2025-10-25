package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/example/shineyshot/internal/appstate"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

type runnable interface{ Run() error }

type root struct {
	fs      *flag.FlagSet
	program string
	state   *appstate.AppState
}

func (r *root) Program() string {
	return r.program
}

func (r *root) FlagSet() *flag.FlagSet {
	return r.fs
}

func newRoot() *root {
	r := &root{fs: flag.NewFlagSet("shineyshot", flag.ExitOnError), program: "shineyshot"}
	return r
}

func (r *root) Run(args []string) error {
	r.fs.Parse(args)
	if r.fs.NArg() < 1 {
		return &UsageError{of: r}
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
	case "interactive":
		cmd = newInteractiveCmd(r)
	case "version":
		cmd = &versionCmd{r: r}
	default:
		return &UsageError{of: r}
	}
	if err != nil {
		return err
	}
	return cmd.Run()
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
