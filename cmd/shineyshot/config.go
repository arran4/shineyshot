package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/shineyshot/internal/config"
)

type configCmd struct {
	*root
	fs     *flag.FlagSet
	output string
}

func parseConfigCmd(args []string, r *root) (*configCmd, error) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	c := &configCmd{root: r, fs: fs}
	fs.Usage = usageFunc(c)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *configCmd) Run() error {
	args := c.fs.Args()
	if len(args) < 1 {
		return &UsageError{of: c}
	}

	subCmd := args[0]
	switch subCmd {
	case "print":
		return c.runPrint()
	case "save":
		return c.runSave()
	default:
		return fmt.Errorf("unknown config command: %s", subCmd)
	}
}

func (c *configCmd) runPrint() error {
	// Print the current configuration
	fmt.Print(c.root.config.String())
	return nil
}

func (c *configCmd) runSave() error {
	cfg := c.root.config
	path := ""

	// If loader found a config file, save there
	// Otherwise determine a default path to save to
	loader := config.NewLoader(version, configPathOverride)
	path = loader.GetConfigPath()

	if path == "" {
		// No existing config found, default to XDG
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home dir: %w", err)
		}
		path = filepath.Join(home, ".config", "shineyshot", "config.rc")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(cfg.String()); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Configuration saved to %s\n", path)
	return nil
}
