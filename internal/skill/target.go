package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

type NeutralTarget struct {
	Scope string
}

func (t *NeutralTarget) Name() string {
	return "neutral"
}

func (t *NeutralTarget) Path() (string, error) {
	if t.Scope == "project" {
		return ".agents/skills", nil
	}

	// User scope fallback
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %w", err)
	}
	return filepath.Join(home, ".agents/skills"), nil
}

type SpecificAgentTarget struct {
	Agent string
	Scope string
}

func (t *SpecificAgentTarget) Name() string {
	return t.Agent
}

func (t *SpecificAgentTarget) Path() (string, error) {
	if t.Scope == "project" {
		return filepath.Join(".agents", t.Agent, "skills"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %w", err)
	}
	return filepath.Join(home, ".agents", t.Agent, "skills"), nil
}

// DefaultTarget returns a neutral target with the specified scope.
func DefaultTarget(scope string) Target {
	return &NeutralTarget{Scope: scope}
}
