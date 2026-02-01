package vcs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGetRepoVCSForPath_Git(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-context")

	// Create initial commit
	h.WriteFile(repoPath, "test.txt", "hello")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Create .beads directory
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	// Check VCS type
	if rv.Type() != VCSTypeGit {
		t.Errorf("expected VCSTypeGit, got %v", rv.Type())
	}

	// Check repo root
	if rv.RepoRoot != repoPath {
		t.Errorf("expected RepoRoot %s, got %s", repoPath, rv.RepoRoot)
	}

	// Check beads dir
	if rv.BeadsDir != beadsDir {
		t.Errorf("expected BeadsDir %s, got %s", beadsDir, rv.BeadsDir)
	}

	// Check helper methods
	if !rv.IsGit() {
		t.Error("IsGit should return true")
	}
	if rv.IsJujutsu() {
		t.Error("IsJujutsu should return false")
	}
}

func TestGetRepoVCSForPath_Jujutsu(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-context")

	// Create .beads directory
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	// Check VCS type
	if rv.Type() != VCSTypeJujutsu {
		t.Errorf("expected VCSTypeJujutsu, got %v", rv.Type())
	}

	// Check helper methods
	if rv.IsGit() {
		t.Error("IsGit should return false")
	}
	if !rv.IsJujutsu() {
		t.Error("IsJujutsu should return true")
	}
}

func TestRepoVCS_SyncCommit_Git(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-sync")

	// Create initial commit
	h.WriteFile(repoPath, "initial.txt", "initial")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Create .beads directory with a file
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}
	h.WriteFile(repoPath, ".beads/issues.jsonl", "{}")

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	ctx := context.Background()

	// Test SyncCommit
	err = rv.SyncCommit(ctx, "wong sync: test commit", ".beads/issues.jsonl")
	if err != nil {
		t.Fatalf("SyncCommit failed: %v", err)
	}

	// Verify commit was created
	log, err := rv.VCS.Log(ctx, 2)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	if len(log) < 2 {
		t.Errorf("expected at least 2 commits, got %d", len(log))
	}

	// Check commit message
	found := false
	for _, entry := range log {
		if entry.Description == "wong sync: test commit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find sync commit in log")
	}
}

func TestRepoVCS_SyncCommit_Jujutsu(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-sync")

	// Create .beads directory with a file
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}
	h.WriteFile(repoPath, ".beads/issues.jsonl", "{}")

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	ctx := context.Background()

	// Test SyncCommit
	err = rv.SyncCommit(ctx, "wong sync: test commit", ".beads/issues.jsonl")
	if err != nil {
		t.Fatalf("SyncCommit failed: %v", err)
	}

	// Verify commit was created
	log, err := rv.VCS.Log(ctx, 5)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// jj should have created changes
	if len(log) < 1 {
		t.Error("expected at least 1 change in log")
	}
}

func TestRepoVCS_Colocated(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateColocatedRepo("colocated-context")

	// Create .beads directory
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	// Should prefer jj in colocated repo
	if !rv.IsJujutsu() {
		t.Error("expected IsJujutsu() to be true for colocated repo")
	}

	// Should report colocated
	if !rv.IsColocated {
		t.Error("expected IsColocated to be true")
	}

	ctx := context.Background()

	// Test GitExport (should not error)
	err = rv.GitExport(ctx)
	if err != nil {
		t.Errorf("GitExport failed: %v", err)
	}
}

func TestRepoVCS_StackInfo(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-stack")

	// Create some changes
	h.WriteFile(repoPath, "file1.txt", "content1")
	h.runCmd(repoPath, "jj", "commit", "-m", "First change")
	h.WriteFile(repoPath, "file2.txt", "content2")
	h.runCmd(repoPath, "jj", "commit", "-m", "Second change")

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	ctx := context.Background()

	// Get stack info
	stack, err := rv.StackInfo(ctx)
	if err != nil {
		t.Fatalf("StackInfo failed: %v", err)
	}

	// Should have changes in stack
	if len(stack) < 2 {
		t.Errorf("expected at least 2 changes in stack, got %d", len(stack))
	}
}

func TestRepoVCS_BeadsPaths(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-paths")

	// Create initial commit
	h.WriteFile(repoPath, "test.txt", "hello")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Create .beads directory
	beadsDir := filepath.Join(repoPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads: %v", err)
	}

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	// Test BeadsJSONLPath
	jsonlPath := rv.BeadsJSONLPath()
	expected := filepath.Join(beadsDir, "issues.jsonl")
	if jsonlPath != expected {
		t.Errorf("expected %s, got %s", expected, jsonlPath)
	}

	// Test BeadsRelPath
	relPath, err := rv.BeadsRelPath(filepath.Join(repoPath, ".beads/issues.jsonl"))
	if err != nil {
		t.Fatalf("BeadsRelPath failed: %v", err)
	}
	if relPath != ".beads/issues.jsonl" {
		t.Errorf("expected .beads/issues.jsonl, got %s", relPath)
	}
}

func TestRepoVCS_Workspace(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-workspace")

	rv, err := GetRepoVCSForPath(repoPath)
	if err != nil {
		t.Fatalf("GetRepoVCSForPath failed: %v", err)
	}

	ctx := context.Background()

	// List workspaces (should have default)
	workspaces, err := rv.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(workspaces))
	}

	// Create workspace
	wsPath := filepath.Join(h.tempDir, "feature-ws")
	err = rv.CreateWorkspace(ctx, "feature", wsPath)
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}

	// Verify
	workspaces, err = rv.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(workspaces))
	}

	// Remove workspace
	err = rv.RemoveWorkspace(ctx, "feature")
	if err != nil {
		t.Fatalf("RemoveWorkspace failed: %v", err)
	}

	// Verify
	workspaces, err = rv.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace after removal, got %d", len(workspaces))
	}
}

func TestResetVCSCaches(t *testing.T) {
	// This test just ensures ResetVCSCaches doesn't panic
	ResetVCSCaches()

	// After reset, next GetRepoVCS should rebuild
	// (We can't test this fully without being in a repo)
}
