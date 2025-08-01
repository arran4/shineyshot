package main

import (
	"bytes"
	"embed"
	"flag"
	"strings"
	"sync"
	"text/template"
)

//go:embed templates/*.txt
var helpFS embed.FS

var (
	helpOnce sync.Once
	helpTmpl map[string]*template.Template
)

func parseHelpTemplates() {
	helpTmpl = make(map[string]*template.Template)
	entries, _ := helpFS.ReadDir("templates")
	for _, e := range entries {
		if e.IsDir() || e.Name() == "helpers.txt" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".txt")
		t := template.Must(template.New(name).ParseFS(helpFS, "templates/helpers.txt", "templates/"+e.Name()))
		helpTmpl[name] = t
	}
}

type flagInfo struct {
	Name     string
	DefValue string
	Usage    string
}

type helpData struct {
	Program string
	Flags   []flagInfo
}

func renderHelp(name string, fs *flag.FlagSet, r *root) string {
	helpOnce.Do(parseHelpTemplates)
	t, ok := helpTmpl[name]
	if !ok {
		return ""
	}
	var flags []flagInfo
	if fs != nil {
		fs.VisitAll(func(f *flag.Flag) {
			flags = append(flags, flagInfo{f.Name, f.DefValue, f.Usage})
		})
	}
	var buf bytes.Buffer
	t.Execute(&buf, helpData{Program: r.program, Flags: flags})
	return buf.String()
}

func rootHelp(r *root) string                       { return renderHelp("root", r.fs, r) }
func annotateHelp(fs *flag.FlagSet, r *root) string { return renderHelp("annotate", fs, r) }
func previewHelp(fs *flag.FlagSet, r *root) string  { return renderHelp("preview", fs, r) }
func snapshotHelp(fs *flag.FlagSet, r *root) string { return renderHelp("snapshot", fs, r) }
func drawHelp(fs *flag.FlagSet, r *root) string     { return renderHelp("draw", fs, r) }
func interactiveHelp(r *root) string                { return renderHelp("interactive", nil, r) }
func versionHelp(r *root) string                    { return renderHelp("version", nil, r) }
