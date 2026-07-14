package skill

import (
	"context"
	"io/fs"
)

// Metadata tracks the provenance and installation details of a skill.
type Metadata struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Revision    string `json:"revision"`
	InstalledAt string `json:"installed_at"`
	AgentTarget string `json:"agent_target"`
	Scope       string `json:"scope"`
}

// Source represents a location from which a skill can be fetched (e.g., local directory, remote repo).
type Source interface {
	// Resolve the source and return a filesystem containing the skill, and the resolved revision/digest.
	Resolve(ctx context.Context) (fs.FS, string, error)
	// CanonicalName returns the canonical source identifier for metadata.
	CanonicalName() string
}

// Target represents a destination directory where skills are installed.
type Target interface {
	// Path returns the base path where skills are installed for this target.
	Path() (string, error)
	// Name returns the identifier of the target (e.g., "codex", "neutral").
	Name() string
}
