package skill

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Installer struct {
	Target Target
}

func NewInstaller(target Target) *Installer {
	return &Installer{Target: target}
}

func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (i *Installer) Install(ctx context.Context, source Source, skillName string) error {
	if !isValidSkillName(skillName) {
		return fmt.Errorf("invalid skill name %q: must contain only alphanumeric characters, hyphens, and underscores", skillName)
	}
	skillFS, revision, err := source.Resolve(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve source: %w", err)
	}

	targetBase, err := i.Target.Path()
	if err != nil {
		return fmt.Errorf("failed to get target path: %w", err)
	}

	skillDest := filepath.Join(targetBase, skillName)

	// Atomic update: install to a temporary directory first
	tmpDest := filepath.Join(targetBase, ".tmp-"+skillName)
	if err := os.MkdirAll(filepath.Dir(tmpDest), 0755); err != nil {
		return fmt.Errorf("failed to create target base directory: %w", err)
	}

	// Remove temp dir if it exists
	_ = os.RemoveAll(tmpDest)

	if err := os.MkdirAll(tmpDest, 0755); err != nil {
		return fmt.Errorf("failed to create tmp dir: %w", err)
	}

	err = fs.WalkDir(skillFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		// Guard against path traversal
		if strings.Contains(path, "..") {
			return fmt.Errorf("invalid path traversal detected: %s", path)
		}

		// Reject symlinks to prevent arbitrary file reading/writing
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed: %s", path)
		}

		destPath := filepath.Join(tmpDest, path)
		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Don't make files executable
		srcFile, err := skillFS.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = srcFile.Close() }()

		destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer func() { _ = destFile.Close() }()

		_, err = io.Copy(destFile, srcFile)
		return err
	})

	if err != nil {
		_ = os.RemoveAll(tmpDest)
		return fmt.Errorf("failed to copy skill files: %w", err)
	}

	// Write metadata
	meta := Metadata{
		Name:        skillName,
		Source:      source.CanonicalName(),
		Revision:    revision,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
		AgentTarget: i.Target.Name(),
	}

	if nt, ok := i.Target.(*NeutralTarget); ok {
		meta.Scope = nt.Scope
	} else if st, ok := i.Target.(*SpecificAgentTarget); ok {
		meta.Scope = st.Scope
	}

	if err := WriteMetadata(tmpDest, meta); err != nil {
		_ = os.RemoveAll(tmpDest)
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	// Atomic rename
	if err := os.RemoveAll(skillDest); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing skill directory: %w", err)
		}
	}

	if err := os.Rename(tmpDest, skillDest); err != nil {
		_ = os.RemoveAll(tmpDest)
		return fmt.Errorf("failed to finalize installation: %w", err)
	}

	return nil
}

func (i *Installer) Remove(skillName string) error {
	if !isValidSkillName(skillName) {
		return fmt.Errorf("invalid skill name %q", skillName)
	}
	targetBase, err := i.Target.Path()
	if err != nil {
		return fmt.Errorf("failed to get target path: %w", err)
	}

	skillDest := filepath.Join(targetBase, skillName)

	// Ensure the directory exists before deleting
	if _, err := os.Stat(skillDest); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill '%s' not found", skillName)
		}
		return fmt.Errorf("failed to stat skill directory: %w", err)
	}

	return os.RemoveAll(skillDest)
}

func (i *Installer) Update(ctx context.Context, skillName string, force bool) (bool, error) {
	if !isValidSkillName(skillName) {
		return false, fmt.Errorf("invalid skill name %q", skillName)
	}
	targetBase, err := i.Target.Path()
	if err != nil {
		return false, fmt.Errorf("failed to get target path: %w", err)
	}

	skillDest := filepath.Join(targetBase, skillName)
	meta, err := ReadMetadata(skillDest)
	if err != nil {
		return false, fmt.Errorf("no source metadata is available, so this skill cannot be updated automatically")
	}

	var source Source
	// Very simple heuristic to detect GitHub sources
	if strings.HasPrefix(meta.Source, "github.com/") {
		parts := strings.Split(meta.Source, "/")
		if len(parts) >= 3 {
			owner := parts[1]
			repo := parts[2]
			path := ""
			if len(parts) > 3 {
				path = strings.Join(parts[3:], "/")
			}
			source = NewGitHubSource(owner, repo, path)
		}
	} else {
		source = NewLocalSource(meta.Source)
	}

	if source == nil {
		return false, fmt.Errorf("unsupported source format: %s", meta.Source)
	}

	_, resolvedRev, err := source.Resolve(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to resolve upstream source: %w", err)
	}

	// Compare revision
	if !force && resolvedRev != "" && resolvedRev == meta.Revision && resolvedRev != "local" {
		return false, nil // Up to date
	}

	// Here, we could also compute a hash of the local folder to check for local modifications,
	// but for simplicity, we just use force.

	err = i.Install(ctx, source, skillName)
	return err == nil, err
}

func (i *Installer) List() ([]Metadata, error) {
	targetBase, err := i.Target.Path()
	if err != nil {
		return nil, fmt.Errorf("failed to get target path: %w", err)
	}

	entries, err := os.ReadDir(targetBase)
	if err != nil {
		if os.IsNotExist(err) {
			return []Metadata{}, nil
		}
		return nil, err
	}

	var skills []Metadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(targetBase, entry.Name())
		meta, err := ReadMetadata(skillPath)
		if err == nil {
			skills = append(skills, meta)
		}
	}
	return skills, nil
}
