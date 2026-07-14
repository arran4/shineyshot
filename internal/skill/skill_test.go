package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSourceResolve(t *testing.T) {
	tmpDir := t.TempDir()

	skillDir := filepath.Join(tmpDir, "myskill")
	err := os.MkdirAll(skillDir, 0755)
	if err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	// Should fail because SKILL.md is missing
	src := NewLocalSource(skillDir)
	_, _, err = src.Resolve(context.Background())
	if err == nil {
		t.Errorf("expected error resolving source without SKILL.md")
	}

	// Create SKILL.md
	err = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0644)
	if err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Should succeed
	_, rev, err := src.Resolve(context.Background())
	if err != nil {
		t.Errorf("failed to resolve source with SKILL.md: %v", err)
	}
	if rev != "local" {
		t.Errorf("expected revision 'local', got %s", rev)
	}
}

func TestInstaller(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a dummy local skill
	skillSrcDir := filepath.Join(tmpDir, "myskill-src")
	_ = os.MkdirAll(skillSrcDir, 0755)
	_ = os.WriteFile(filepath.Join(skillSrcDir, "SKILL.md"), []byte("# Dummy Skill"), 0644)
	_ = os.WriteFile(filepath.Join(skillSrcDir, "extra.txt"), []byte("extra"), 0644)

	src := NewLocalSource(skillSrcDir)

	targetBase := filepath.Join(tmpDir, "agents-skills")
	_ = os.MkdirAll(targetBase, 0755)

	// Create a dummy target
	target := &dummyTarget{path: targetBase, name: "dummy"}

	installer := NewInstaller(target)

	// Test Install
	err := installer.Install(context.Background(), src, "myskill")
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify files
	destDir := filepath.Join(targetBase, "myskill")
	if _, err := os.Stat(filepath.Join(destDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not found in dest")
	}
	if _, err := os.Stat(filepath.Join(destDir, "extra.txt")); err != nil {
		t.Errorf("extra.txt not found in dest")
	}

	// Test List
	skills, err := installer.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "myskill" {
		t.Errorf("expected skill 'myskill', got %s", skills[0].Name)
	}
	if skills[0].AgentTarget != "dummy" {
		t.Errorf("expected target 'dummy', got %s", skills[0].AgentTarget)
	}

	// Test Remove
	err = installer.Remove("myskill")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, err := os.Stat(destDir); !os.IsNotExist(err) {
		t.Errorf("dest dir still exists after remove")
	}
}

// dummyTarget for testing
type dummyTarget struct {
	path string
	name string
}

func (t *dummyTarget) Path() (string, error) { return t.path, nil }
func (t *dummyTarget) Name() string          { return t.name }
