package skill

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitHubSource struct {
	owner string
	repo  string
	path  string
}

func NewGitHubSource(owner, repo, path string) *GitHubSource {
	return &GitHubSource{owner: owner, repo: repo, path: path}
}

func (s *GitHubSource) CanonicalName() string {
	return fmt.Sprintf("github.com/%s/%s/%s", s.owner, s.repo, s.path)
}

func (s *GitHubSource) Resolve(ctx context.Context) (fs.FS, string, error) {
	// For simplicity, we use git to clone the repository to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "shineyshot-skill-*")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", s.owner, s.repo)

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", repoURL, tmpDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("git clone failed: %v\nOutput: %s", err, string(out))
	}

	// Get the commit hash
	cmdRev := exec.CommandContext(ctx, "git", "-C", tmpDir, "rev-parse", "HEAD")
	revOut, err := cmdRev.Output()
	var revision string
	if err == nil {
		revision = strings.TrimSpace(string(revOut))
	} else {
		revision = "unknown"
	}

	targetPath := tmpDir
	if s.path != "" {
		targetPath = filepath.Join(tmpDir, s.path)
	}

	// Ensure SKILL.md exists
	skillMdPath := filepath.Join(targetPath, "SKILL.md")
	_, err = os.Stat(skillMdPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", fmt.Errorf("SKILL.md not found in repository path")
	}

	return os.DirFS(targetPath), revision, nil
}
