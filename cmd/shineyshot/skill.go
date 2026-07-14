package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/example/shineyshot/internal/skill"
)

type skillCmd struct {
	r          *root
	subcommand string
	args       []string
}

func parseSkillCmd(args []string, r *root) (runnable, error) {
	if len(args) < 1 {
		return nil, &UsageError{of: &skillCmd{r: r}}
	}
	return &skillCmd{r: r, subcommand: args[0], args: args[1:]}, nil
}

func (c *skillCmd) Program() string {
	return c.r.Program() + " skill"
}

func (c *skillCmd) Template() string {
	return "skill.txt"
}

func (c *skillCmd) FlagSet() *flag.FlagSet {
	return nil
}

func (c *skillCmd) Run() error {
	switch c.subcommand {
	case "install":
		return c.runInstall()
	case "update":
		return c.runUpdate()
	case "remove":
		return c.runRemove()
	case "list":
		return c.runList()
	case "inspect":
		return c.runInspect()
	default:
		return &UsageError{of: c}
	}
}

func (c *skillCmd) resolveTarget(scope, agent string) skill.Target {
	if agent != "" {
		return &skill.SpecificAgentTarget{Agent: agent, Scope: scope}
	}
	return skill.DefaultTarget(scope)
}

func (c *skillCmd) runInstall() error {
	fs := flag.NewFlagSet("skill install", flag.ExitOnError)
	var (
		scope string
		agent string
	)
	fs.StringVar(&scope, "scope", "user", "installation scope (user, project)")
	fs.StringVar(&agent, "agent", "", "target agent (codex, claude, copilot, cursor)")
	fs.Usage = usageFunc(&skillInstallCmd{c, fs})

	if err := fs.Parse(c.args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return &UsageError{of: &skillInstallCmd{c, fs}}
	}

	sourceStr := fs.Arg(0)
	skillName := ""
	if fs.NArg() >= 2 {
		skillName = fs.Arg(1)
	}

	var src skill.Source
	if strings.Contains(sourceStr, "/") && !strings.HasPrefix(sourceStr, ".") && !strings.HasPrefix(sourceStr, "/") {
		// GitHub source format owner/repo[/path]
		parts := strings.Split(sourceStr, "/")
		owner := parts[0]
		repo := parts[1]
		path := ""
		if len(parts) > 2 {
			path = strings.Join(parts[2:], "/")
		}
		src = skill.NewGitHubSource(owner, repo, path)
		if skillName == "" {
			skillName = repo
		}
	} else {
		// Local source
		src = skill.NewLocalSource(sourceStr)
		if skillName == "" {
			// Extract last directory name
			skillName = strings.TrimSuffix(sourceStr, "/")
			if idx := strings.LastIndex(skillName, "/"); idx != -1 {
				skillName = skillName[idx+1:]
			}
		}
	}

	if skillName == "" {
		skillName = "default"
	}

	target := c.resolveTarget(scope, agent)
	installer := skill.NewInstaller(target)

	_, _ = fmt.Fprintf(os.Stdout, "Installing skill '%s'...\n", skillName)
	err := installer.Install(context.Background(), src, skillName)
	if err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	_, _ = fmt.Fprintf(os.Stdout, "Skill '%s' installed successfully.\n", skillName)
	return nil
}

func (c *skillCmd) runUpdate() error {
	fs := flag.NewFlagSet("skill update", flag.ExitOnError)
	var (
		scope string
		agent string
		force bool
	)
	fs.StringVar(&scope, "scope", "user", "installation scope (user, project)")
	fs.StringVar(&agent, "agent", "", "target agent")
	fs.BoolVar(&force, "force", false, "force update even if up to date")
	fs.Usage = usageFunc(&skillUpdateCmd{c, fs})

	if err := fs.Parse(c.args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return &UsageError{of: &skillUpdateCmd{c, fs}}
	}

	skillName := fs.Arg(0)
	target := c.resolveTarget(scope, agent)
	installer := skill.NewInstaller(target)

	_, _ = fmt.Fprintf(os.Stdout, "Updating skill '%s'...\n", skillName)
	updated, err := installer.Update(context.Background(), skillName, force)
	if err != nil {
		return err
	}

	if updated {
		fmt.Fprintf(os.Stdout, "Skill '%s' updated successfully.\n", skillName)
	} else {
		fmt.Fprintf(os.Stdout, "Skill '%s' is already up to date.\n", skillName)
	}
	return nil
}

func (c *skillCmd) runRemove() error {
	fs := flag.NewFlagSet("skill remove", flag.ExitOnError)
	var (
		scope string
		agent string
	)
	fs.StringVar(&scope, "scope", "user", "installation scope")
	fs.StringVar(&agent, "agent", "", "target agent")
	fs.Usage = usageFunc(&skillRemoveCmd{c, fs})

	if err := fs.Parse(c.args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return &UsageError{of: &skillRemoveCmd{c, fs}}
	}

	skillName := fs.Arg(0)
	target := c.resolveTarget(scope, agent)
	installer := skill.NewInstaller(target)

	err := installer.Remove(skillName)
	if err != nil {
		return fmt.Errorf("failed to remove skill: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Skill '%s' removed from scope '%s'.\n", skillName, scope)
	return nil
}

func (c *skillCmd) runList() error {
	fs := flag.NewFlagSet("skill list", flag.ExitOnError)
	var (
		scope  string
		agent  string
		format string
	)
	fs.StringVar(&scope, "scope", "user", "installation scope")
	fs.StringVar(&agent, "agent", "", "target agent")
	fs.StringVar(&format, "format", "", "output format (json)")
	fs.Usage = usageFunc(&skillListCmd{c, fs})

	if err := fs.Parse(c.args); err != nil {
		return err
	}

	target := c.resolveTarget(scope, agent)
	installer := skill.NewInstaller(target)

	skills, err := installer.List()
	if err != nil {
		return fmt.Errorf("failed to list skills: %w", err)
	}

	if format == "json" {
		b, err := json.MarshalIndent(skills, "", "  ")
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(os.Stdout, string(b))
		return nil
	}

	if len(skills) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No skills installed.")
		return nil
	}

	fmt.Fprintf(os.Stdout, "%-20s %-15s %-30s %s\n", "NAME", "SCOPE", "SOURCE", "REVISION")
	for _, s := range skills {
		fmt.Fprintf(os.Stdout, "%-20s %-15s %-30s %s\n", s.Name, s.Scope, s.Source, s.Revision)
	}
	return nil
}

func (c *skillCmd) runInspect() error {
	fs := flag.NewFlagSet("skill inspect", flag.ExitOnError)
	var (
		scope string
		agent string
	)
	fs.StringVar(&scope, "scope", "user", "installation scope")
	fs.StringVar(&agent, "agent", "", "target agent")
	fs.Usage = usageFunc(&skillInspectCmd{c, fs})

	if err := fs.Parse(c.args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return &UsageError{of: &skillInspectCmd{c, fs}}
	}

	skillName := fs.Arg(0)
	target := c.resolveTarget(scope, agent)
	installer := skill.NewInstaller(target)

	skills, err := installer.List()
	if err != nil {
		return err
	}

	for _, s := range skills {
		if s.Name == skillName {
			b, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal skill details: %w", err)
			}
			_, _ = fmt.Fprintln(os.Stdout, string(b))
			return nil
		}
	}

	return fmt.Errorf("skill '%s' not found", skillName)
}

// Help wrappers for subcommands
type skillInstallCmd struct {
	*skillCmd
	fs *flag.FlagSet
}

func (c *skillInstallCmd) FlagSet() *flag.FlagSet { return c.fs }
func (c *skillInstallCmd) Template() string       { return "skill_install.txt" }

type skillUpdateCmd struct {
	*skillCmd
	fs *flag.FlagSet
}

func (c *skillUpdateCmd) FlagSet() *flag.FlagSet { return c.fs }
func (c *skillUpdateCmd) Template() string       { return "skill_update.txt" }

type skillRemoveCmd struct {
	*skillCmd
	fs *flag.FlagSet
}

func (c *skillRemoveCmd) FlagSet() *flag.FlagSet { return c.fs }
func (c *skillRemoveCmd) Template() string       { return "skill_remove.txt" }

type skillListCmd struct {
	*skillCmd
	fs *flag.FlagSet
}

func (c *skillListCmd) FlagSet() *flag.FlagSet { return c.fs }
func (c *skillListCmd) Template() string       { return "skill_list.txt" }

type skillInspectCmd struct {
	*skillCmd
	fs *flag.FlagSet
}

func (c *skillInspectCmd) FlagSet() *flag.FlagSet { return c.fs }
func (c *skillInspectCmd) Template() string       { return "skill_inspect.txt" }
