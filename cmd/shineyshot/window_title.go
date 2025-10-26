package main

import (
	"fmt"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
)

type titleOptions struct {
	File       string
	Mode       string
	Detail     string
	Tab        string
	LastSaved  string
	Background string
	Extras     []string
}

func windowTitle(opts titleOptions) string {
	parts := []string{appstate.ProgramTitle}

	file := strings.TrimSpace(opts.File)
	if file != "" {
		parts = append(parts, file)
	}

	mode := strings.TrimSpace(opts.Mode)
	if mode != "" {
		parts = append(parts, mode)
	}

	detail := strings.TrimSpace(opts.Detail)
	if detail != "" {
		parts = append(parts, detail)
	}

	tab := strings.TrimSpace(opts.Tab)
	if tab != "" {
		parts = append(parts, tab)
	}

	extras := make([]string, 0, len(opts.Extras)+4)

	saved := strings.TrimSpace(opts.LastSaved)
	if saved != "" {
		extras = append(extras, fmt.Sprintf("last saved %s", saved))
	}

	if strings.TrimSpace(version) != "" {
		extras = append(extras, fmt.Sprintf("v%s", strings.TrimSpace(version)))
	}

	if strings.TrimSpace(commit) != "" {
		extras = append(extras, fmt.Sprintf("commit %s", strings.TrimSpace(commit)))
	}

	if strings.TrimSpace(date) != "" {
		extras = append(extras, strings.TrimSpace(date))
	}

	background := strings.TrimSpace(opts.Background)
	if background != "" {
		extras = append(extras, fmt.Sprintf("session %s", background))
	}

	if len(opts.Extras) > 0 {
		extras = append(extras, opts.Extras...)
	}

	if len(extras) > 0 {
		parts = append(parts, extras...)
	}

	return strings.Join(parts, " - ")
}
