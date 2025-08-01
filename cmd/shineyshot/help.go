package main

import (
	"bytes"
	"embed"
	"flag"
	"log"
	"sync"
	"text/template"
)

//go:embed templates/*.txt
var helpFS embed.FS

var (
	helpOnce sync.Once
	helpTmpl *template.Template
)

func parseHelpTemplates() {
	helpTmpl = template.Must(template.New("").Funcs(map[string]any{
		"flags": func(fs *flag.FlagSet) []flagInfo {
			result := []flagInfo{}
			fs.VisitAll(func(f *flag.Flag) {
				result = append(result, flagInfo{f.Name, f.DefValue, f.Usage})
			})
			return result
		},
	}).ParseFS(helpFS, "templates/*.txt"))
}

type flagInfo struct {
	Name     string
	DefValue string
	Usage    string
}

type HelpData interface {
	Program() string
	Template() string
	FlagSet() *flag.FlagSet
}

type UsageError struct {
	of HelpData
}

func (e *UsageError) Error() string {
	help, err := e.renderHelp()
	if err != nil {
		return err.Error()
	}
	return help
}

func (e *UsageError) renderHelp() (string, error) {
	helpOnce.Do(parseHelpTemplates)
	var flags []flagInfo
	if e.of.FlagSet() != nil {
		e.of.FlagSet().VisitAll(func(f *flag.Flag) {
			flags = append(flags, flagInfo{f.Name, f.DefValue, f.Usage})
		})
	}
	var buf bytes.Buffer
	err := helpTmpl.ExecuteTemplate(&buf, e.of.Template(), e.of)
	if err != nil {
		log.Printf("error rendering help template: %v", err)
		return "", err
	}
	return buf.String(), nil
}

func (r *root) Template() string {
	return "root.txt"
}

func (a *annotateCmd) Template() string {
	return "annotate.txt"
}

func (p *previewCmd) Template() string {
	return "preview.txt"
}

func (s *snapshotCmd) Template() string {
	return "snapshot.txt"
}

func (d *drawCmd) Template() string {
	return "draw.txt"
}

func (i *interactiveCmd) Template() string {
	return "interactive.txt"
}

func (v *versionCmd) Template() string {
	return "version.txt"
}
