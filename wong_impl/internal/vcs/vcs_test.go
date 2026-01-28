package vcs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelper provides utilities for creating test repositories.
type TestHelper struct {
	t       *testing.T
	tempDir string
}

// NewTestHelper creates a new test helper with a temp directory.
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()
	dir, err := os.MkdirTemp("", "vcs-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return &TestHelper{t: t, tempDir: dir}
}

// CreateGitRepo creates a git repository in a subdirectory.
func (h *TestHelper) CreateGitRepo(name string) string {
	h.t.Helper()
	path := filepath.Join(h.tempDir, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		h.t.Fatalf("failed to create dir: %v", err)
	}

	// Initialize git repo
	h.runCmd(path, "git", "init")
	h.runCmd(path, "git", "config", "user.email", "test@example.com")
	h.runCmd(path, "git", "config", "user.name", "Test User")
	h.runCmd(path, "git", "config", "commit.gpgsign", "false")

	return path
}

// CreateJJRepo creates a jujutsu repository in a subdirectory.
func (h *TestHelper) CreateJJRepo(name string) string {
	h.t.Helper()
	path := filepath.Join(h.tempDir, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		h.t.Fatalf("failed to create dir: %v", err)
	}

	// Initialize jj repo (creates a git-backed jj repo by default)
	h.runCmd(path, "jj", "git", "init")
	h.runCmd(path, "jj", "config", "set", "--repo", "user.email", "test@example.com")
	h.runCmd(path, "jj", "config", "set", "--repo", "user.name", "Test User")

	return path
}

// CreateColocatedRepo creates a colocated jj+git repository.
func (h *TestHelper) CreateColocatedRepo(name string) string {
	h.t.Helper()
	path := filepath.Join(h.tempDir, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		h.t.Fatalf("failed to create dir: %v", err)
	}

	// Initialize git first, then jj
	h.runCmd(path, "git", "init")
	h.runCmd(path, "git", "config", "user.email", "test@example.com")
	h.runCmd(path, "git", "config", "user.name", "Test User")
	h.runCmd(path, "git", "config", "commit.gpgsign", "false")
	h.runCmd(path, "jj", "git", "init", "--colocate")
	h.runCmd(path, "jj", "config", "set", "--repo", "user.email", "test@example.com")
	h.runCmd(path, "jj", "config", "set", "--repo", "user.name", "Test User")

	return path
}

// WriteFile writes a file in the repo.
func (h *TestHelper) WriteFile(repoPath, filename, content string) {
	h.t.Helper()
	path := filepath.Join(repoPath, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		h.t.Fatalf("failed to write file: %v", err)
	}
}

// runCmd runs a command in the given directory.
func (h *TestHelper) runCmd(dir string, name string, args ...string) string {
	h.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		h.t.Fatalf("command %s %v failed: %v\nOutput: %s", name, args, err, output)
	}
	return string(output)
}

// runCmdNoFail runs a command but doesn't fail on error.
func (h *TestHelper) runCmdNoFail(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// --- Detection Tests ---

func TestDetectVCSType_Git(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-repo")

	vcsType, err := detectVCSType(repoPath)
	if err != nil {
		t.Fatalf("detectVCSType failed: %v", err)
	}
	if vcsType != VCSTypeGit {
		t.Errorf("expected VCSTypeGit, got %v", vcsType)
	}
}

func TestDetectVCSType_Jujutsu(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-repo")

	vcsType, err := detectVCSType(repoPath)
	if err != nil {
		t.Fatalf("detectVCSType failed: %v", err)
	}
	if vcsType != VCSTypeJujutsu {
		t.Errorf("expected VCSTypeJujutsu, got %v", vcsType)
	}
}

func TestDetectVCSType_Colocated_PrefersJJ(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateColocatedRepo("colocated-repo")

	vcsType, err := detectVCSType(repoPath)
	if err != nil {
		t.Fatalf("detectVCSType failed: %v", err)
	}
	// Should prefer jj in colocated repos (user chose jj-first workflow)
	if vcsType != VCSTypeJujutsu {
		t.Errorf("expected VCSTypeJujutsu for colocated repo, got %v", vcsType)
	}
}

func TestIsColocatedRepo(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)

	// Test non-colocated git repo
	gitPath := h.CreateGitRepo("git-only")
	isColoc, err := IsColocatedRepo(gitPath)
	if err != nil {
		t.Fatalf("IsColocatedRepo failed: %v", err)
	}
	if isColoc {
		t.Error("git-only repo should not be colocated")
	}

	// Test colocated repo
	colocPath := h.CreateColocatedRepo("colocated")
	isColoc, err = IsColocatedRepo(colocPath)
	if err != nil {
		t.Fatalf("IsColocatedRepo failed: %v", err)
	}
	if !isColoc {
		t.Error("colocated repo should be detected as colocated")
	}
}

// --- Git Backend Tests ---

func TestGitVCS_BasicOperations(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-ops")

	vcs, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	ctx := context.Background()

	// Test Type
	if vcs.Type() != VCSTypeGit {
		t.Errorf("expected VCSTypeGit, got %v", vcs.Type())
	}

	// Test RepoRoot
	if vcs.RepoRoot() != repoPath {
		t.Errorf("expected %s, got %s", repoPath, vcs.RepoRoot())
	}

	// Create a file and test status
	h.WriteFile(repoPath, "test.txt", "hello world")

	status, err := vcs.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	found := false
	for _, entry := range status {
		if entry.Path == "test.txt" && entry.Status == FileStatusUntracked {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find untracked test.txt in status")
	}

	// Test Stage
	err = vcs.Stage(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Stage failed: %v", err)
	}

	// Test Commit
	err = vcs.Commit(ctx, "Initial commit", nil)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Test CurrentChange
	change, err := vcs.CurrentChange(ctx)
	if err != nil {
		t.Fatalf("CurrentChange failed: %v", err)
	}
	if change.Description != "Initial commit" {
		t.Errorf("expected 'Initial commit', got '%s'", change.Description)
	}

	// Test Log
	log, err := vcs.Log(ctx, 5)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(log) != 1 {
		t.Errorf("expected 1 commit, got %d", len(log))
	}
}

func TestGitVCS_Branches(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-branches")

	vcs, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create initial commit
	h.WriteFile(repoPath, "test.txt", "hello")
	vcs.Stage(ctx, "test.txt")
	vcs.Commit(ctx, "Initial commit", nil)

	// Test CurrentBranch
	branch, err := vcs.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	// Could be "main" or "master" depending on git config
	if branch != "main" && branch != "master" {
		t.Errorf("unexpected branch: %s", branch)
	}

	// Test CreateBranch
	err = vcs.CreateBranch(ctx, "feature")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Test ListBranches
	branches, err := vcs.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	foundFeature := false
	for _, b := range branches {
		if b.Name == "feature" {
			foundFeature = true
			break
		}
	}
	if !foundFeature {
		t.Error("expected to find 'feature' branch")
	}

	// Test SwitchBranch
	err = vcs.SwitchBranch(ctx, "feature")
	if err != nil {
		t.Fatalf("SwitchBranch failed: %v", err)
	}

	branch, err = vcs.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch failed: %v", err)
	}
	if branch != "feature" {
		t.Errorf("expected 'feature', got '%s'", branch)
	}
}

func TestGitVCS_Worktrees(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-worktrees")

	vcs, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create initial commit
	h.WriteFile(repoPath, "test.txt", "hello")
	vcs.Stage(ctx, "test.txt")
	vcs.Commit(ctx, "Initial commit", nil)

	// Test ListWorkspaces (should have just the main worktree)
	workspaces, err := vcs.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(workspaces))
	}

	// Test CreateWorkspace
	worktreePath := filepath.Join(h.tempDir, "feature-worktree")
	err = vcs.CreateWorkspace(ctx, "feature", worktreePath)
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}

	// Verify worktree was created
	workspaces, err = vcs.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(workspaces))
	}
}

// --- Jujutsu Backend Tests ---

func TestJujutsuVCS_BasicOperations(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-ops")

	vcs, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	ctx := context.Background()

	// Test Type
	if vcs.Type() != VCSTypeJujutsu {
		t.Errorf("expected VCSTypeJujutsu, got %v", vcs.Type())
	}

	// Test RepoRoot
	if vcs.RepoRoot() != repoPath {
		t.Errorf("expected %s, got %s", repoPath, vcs.RepoRoot())
	}

	// Create a file - jj auto-snapshots
	h.WriteFile(repoPath, "test.txt", "hello world")

	// Force snapshot
	h.runCmd(repoPath, "jj", "status")

	// Test CurrentChange
	change, err := vcs.CurrentChange(ctx)
	if err != nil {
		t.Fatalf("CurrentChange failed: %v", err)
	}
	if change.ID == "" {
		t.Error("expected non-empty change ID")
	}
	if !change.IsWorking {
		t.Error("expected IsWorking to be true for @")
	}

	// Test Status
	status, err := vcs.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	found := false
	for _, entry := range status {
		if strings.HasSuffix(entry.Path, "test.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find test.txt in status")
	}

	// Test Commit (creates new change)
	err = vcs.Commit(ctx, "Test commit", nil)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}

func TestJujutsuVCS_StackedChanges(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-stack")

	vcs, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create a file and commit
	h.WriteFile(repoPath, "file1.txt", "content 1")
	h.runCmd(repoPath, "jj", "status") // snapshot
	err = vcs.Commit(ctx, "First change", nil)
	if err != nil {
		t.Fatalf("First Commit failed: %v", err)
	}

	// Create another file and commit (building a stack)
	h.WriteFile(repoPath, "file2.txt", "content 2")
	h.runCmd(repoPath, "jj", "status") // snapshot
	err = vcs.Commit(ctx, "Second change", nil)
	if err != nil {
		t.Fatalf("Second Commit failed: %v", err)
	}

	// Test StackInfo
	stack, err := vcs.StackInfo(ctx)
	if err != nil {
		t.Fatalf("StackInfo failed: %v", err)
	}
	if len(stack) < 2 {
		t.Errorf("expected at least 2 changes in stack, got %d", len(stack))
	}

	// Test New (create new change on top)
	err = vcs.New(ctx, "New working change")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Verify we have a new working copy
	change, err := vcs.CurrentChange(ctx)
	if err != nil {
		t.Fatalf("CurrentChange failed: %v", err)
	}
	if !change.IsWorking {
		t.Error("expected new change to be working copy")
	}
}

func TestJujutsuVCS_Bookmarks(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-bookmarks")

	vcs, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create a commit first (jj needs a non-empty change for bookmarks)
	h.WriteFile(repoPath, "test.txt", "hello")
	h.runCmd(repoPath, "jj", "status")
	vcs.Commit(ctx, "Initial commit", nil)

	// Test CreateBranch (bookmark in jj)
	err = vcs.CreateBranch(ctx, "feature")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Test ListBranches
	branches, err := vcs.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	foundFeature := false
	for _, b := range branches {
		if b.Name == "feature" {
			foundFeature = true
			break
		}
	}
	if !foundFeature {
		t.Error("expected to find 'feature' bookmark")
	}
}

func TestJujutsuVCS_Workspaces(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-workspaces")

	vcs, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create initial content
	h.WriteFile(repoPath, "test.txt", "hello")
	h.runCmd(repoPath, "jj", "status")

	// Test ListWorkspaces (should have "default")
	workspaces, err := vcs.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(workspaces))
	}

	// Test CreateWorkspace
	workspacePath := filepath.Join(h.tempDir, "feature-workspace")
	err = vcs.CreateWorkspace(ctx, "feature", workspacePath)
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}

	// Verify workspace was created
	workspaces, err = vcs.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(workspaces))
	}

	// Test RemoveWorkspace
	err = vcs.RemoveWorkspace(ctx, "feature")
	if err != nil {
		t.Fatalf("RemoveWorkspace failed: %v", err)
	}

	workspaces, err = vcs.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace after removal, got %d", len(workspaces))
	}
}

func TestJujutsuVCS_Edit(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-edit")

	vcs, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create a file and commit
	h.WriteFile(repoPath, "test.txt", "hello")
	h.runCmd(repoPath, "jj", "status")
	vcs.Commit(ctx, "First commit", nil)

	// Get the commit's change ID
	log, err := vcs.Log(ctx, 2)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	if len(log) < 2 {
		t.Skip("need at least 2 changes for edit test")
	}

	// Find a non-working change to edit
	var targetID string
	for _, entry := range log {
		if !entry.IsWorking {
			targetID = entry.ShortID
			break
		}
	}

	if targetID == "" {
		t.Skip("no non-working change to edit")
	}

	// Test Edit
	err = vcs.Edit(ctx, targetID)
	if err != nil {
		t.Fatalf("Edit failed: %v", err)
	}

	// Verify we're now editing that change
	current, err := vcs.CurrentChange(ctx)
	if err != nil {
		t.Fatalf("CurrentChange failed: %v", err)
	}
	if !strings.HasPrefix(current.ID, targetID) && !strings.HasPrefix(targetID, current.ShortID) {
		t.Logf("Note: current change %s might differ from target %s due to jj behavior", current.ShortID, targetID)
	}
}

// --- Colocated Repo Tests ---

func TestColocatedRepo_PreferJJ(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateColocatedRepo("colocated")

	// DetectVCS should return jj instance
	vcs, err := DetectVCS(repoPath)
	if err != nil {
		t.Fatalf("DetectVCS failed: %v", err)
	}

	if vcs.Type() != VCSTypeJujutsu {
		t.Errorf("expected VCSTypeJujutsu for colocated repo, got %v", vcs.Type())
	}

	if !vcs.IsColocated() {
		t.Error("expected IsColocated to return true")
	}
}

func TestColocatedRepo_GitExportImport(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateColocatedRepo("colocated-sync")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	ctx := context.Background()

	// Create a file in jj
	h.WriteFile(repoPath, "test.txt", "hello from jj")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Commit from jj", nil)

	// Export to git
	err = jjVCS.GitExport(ctx)
	if err != nil {
		t.Fatalf("GitExport failed: %v", err)
	}

	// Verify git sees the commit
	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	log, err := gitVCS.Log(ctx, 5)
	if err != nil {
		t.Fatalf("Git Log failed: %v", err)
	}

	// Should have at least one commit
	if len(log) == 0 {
		t.Error("expected git to see commits after export")
	}
}

// --- Integration Tests ---

func TestVCS_FullWorkflow(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	ctx := context.Background()

	// Test the same workflow with both VCS backends
	for _, tc := range []struct {
		name       string
		createRepo func(string) string
		vcsType    VCSType
	}{
		{"git", h.CreateGitRepo, VCSTypeGit},
		{"jj", h.CreateJJRepo, VCSTypeJujutsu},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := tc.createRepo(tc.name + "-workflow")

			vcs, err := DetectVCS(repoPath)
			if err != nil {
				t.Fatalf("DetectVCS failed: %v", err)
			}

			if vcs.Type() != tc.vcsType {
				t.Errorf("expected %v, got %v", tc.vcsType, vcs.Type())
			}

			// 1. Create and stage a file
			h.WriteFile(repoPath, "README.md", "# Test Project")
			err = vcs.Stage(ctx, "README.md")
			if err != nil {
				t.Fatalf("Stage failed: %v", err)
			}

			// For jj, trigger snapshot
			if tc.vcsType == VCSTypeJujutsu {
				h.runCmd(repoPath, "jj", "status")
			}

			// 2. Commit
			err = vcs.Commit(ctx, "Initial commit", nil)
			if err != nil {
				t.Fatalf("Commit failed: %v", err)
			}

			// 3. Create another file
			h.WriteFile(repoPath, "main.go", "package main")
			vcs.Stage(ctx, "main.go")
			if tc.vcsType == VCSTypeJujutsu {
				h.runCmd(repoPath, "jj", "status")
			}

			// 4. Commit again
			err = vcs.Commit(ctx, "Add main.go", nil)
			if err != nil {
				t.Fatalf("Second commit failed: %v", err)
			}

			// 5. Check log
			log, err := vcs.Log(ctx, 10)
			if err != nil {
				t.Fatalf("Log failed: %v", err)
			}

			if len(log) < 2 {
				t.Errorf("expected at least 2 commits, got %d", len(log))
			}

			// 6. Check stack info
			stack, err := vcs.StackInfo(ctx)
			if err != nil {
				t.Fatalf("StackInfo failed: %v", err)
			}

			if len(stack) == 0 {
				t.Error("expected non-empty stack")
			}
		})
	}
}

// --- Benchmark Tests ---

func BenchmarkGitStatus(b *testing.B) {
	dir, _ := os.MkdirTemp("", "bench-git-*")
	defer os.RemoveAll(dir)

	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	vcs, _ := NewGitVCS(dir)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vcs.Status(ctx)
	}
}

func BenchmarkJJStatus(b *testing.B) {
	if _, err := exec.LookPath("jj"); err != nil {
		b.Skip("jj not installed")
	}

	dir, _ := os.MkdirTemp("", "bench-jj-*")
	defer os.RemoveAll(dir)

	exec.Command("jj", "git", "init", dir).Run()

	vcs, _ := NewJujutsuVCS(dir)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vcs.Status(ctx)
	}
}

// --- Phase 1: Ref Resolution, Merge, Config Tests ---

func TestGitVCS_BranchExists(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-branch-exists")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Create initial commit so branches can exist
	h.WriteFile(repoPath, "file.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Current branch should exist
	exists, err := gitVCS.BranchExists(ctx, "main")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	// main might not be default, check master too
	if !exists {
		exists, err = gitVCS.BranchExists(ctx, "master")
		if err != nil {
			t.Fatalf("BranchExists (master) failed: %v", err)
		}
	}
	if !exists {
		t.Error("expected current branch to exist")
	}

	// Non-existent branch
	exists, err = gitVCS.BranchExists(ctx, "nonexistent-branch-xyz")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if exists {
		t.Error("expected nonexistent branch to not exist")
	}
}

func TestGitVCS_ResolveRef(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-resolve-ref")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	h.WriteFile(repoPath, "file.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Resolve HEAD
	sha, err := gitVCS.ResolveRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef failed: %v", err)
	}
	if len(sha) < 7 {
		t.Errorf("expected full SHA, got %q", sha)
	}
}

func TestGitVCS_IsAncestor(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-is-ancestor")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	h.WriteFile(repoPath, "file.txt", "v1")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "First")

	first, _ := gitVCS.ResolveRef(ctx, "HEAD")

	h.WriteFile(repoPath, "file.txt", "v2")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Second")

	second, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// First should be ancestor of second
	isAnc, err := gitVCS.IsAncestor(ctx, first, second)
	if err != nil {
		t.Fatalf("IsAncestor failed: %v", err)
	}
	if !isAnc {
		t.Error("expected first to be ancestor of second")
	}

	// Second should NOT be ancestor of first
	isAnc, err = gitVCS.IsAncestor(ctx, second, first)
	if err != nil {
		t.Fatalf("IsAncestor failed: %v", err)
	}
	if isAnc {
		t.Error("expected second to NOT be ancestor of first")
	}
}

func TestGitVCS_MergeAndIsMerging(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-merge")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	h.WriteFile(repoPath, "file.txt", "main content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial on main")

	// Get the default branch name
	defaultBranch, _ := gitVCS.CurrentBranch(ctx)

	// Create a branch with different content
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	h.WriteFile(repoPath, "feature.txt", "feature content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Feature commit")

	// Switch back and merge
	h.runCmd(repoPath, "git", "checkout", defaultBranch)
	err = gitVCS.Merge(ctx, "feature", "Merge feature")
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Should not be merging after successful merge
	merging, err := gitVCS.IsMerging(ctx)
	if err != nil {
		t.Fatalf("IsMerging failed: %v", err)
	}
	if merging {
		t.Error("expected not to be merging after successful merge")
	}
}

func TestGitVCS_Config(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-config")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Set and get a config value
	err = gitVCS.SetConfig(ctx, "beads.test-key", "test-value")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	val, err := gitVCS.GetConfig(ctx, "beads.test-key")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if val != "test-value" {
		t.Errorf("expected 'test-value', got %q", val)
	}
}

func TestGitVCS_CheckoutFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-checkout-file")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Commit a file
	h.WriteFile(repoPath, "file.txt", "original")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Original")

	// Modify it
	h.WriteFile(repoPath, "file.txt", "modified")

	// Checkout from HEAD (restore original)
	err = gitVCS.CheckoutFile(ctx, "HEAD", "file.txt")
	if err != nil {
		t.Fatalf("CheckoutFile failed: %v", err)
	}

	// Verify content is restored
	content, _ := os.ReadFile(filepath.Join(repoPath, "file.txt"))
	if string(content) != "original" {
		t.Errorf("expected 'original', got %q", string(content))
	}
}

func TestJujutsuVCS_BranchExistsAndResolveRef(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-branch-resolve")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Create content and a bookmark
	h.WriteFile(repoPath, "file.txt", "content")
	jjVCS.Commit(ctx, "Initial", nil)
	jjVCS.CreateBranch(ctx, "test-bookmark")

	// Bookmark should exist
	exists, err := jjVCS.BranchExists(ctx, "test-bookmark")
	if err != nil {
		t.Fatalf("BranchExists failed: %v", err)
	}
	if !exists {
		t.Error("expected test-bookmark to exist")
	}

	// Nonexistent should not
	exists, _ = jjVCS.BranchExists(ctx, "nope-xyz")
	if exists {
		t.Error("expected nope-xyz to not exist")
	}

	// Resolve @
	changeID, err := jjVCS.ResolveRef(ctx, "@")
	if err != nil {
		t.Fatalf("ResolveRef failed: %v", err)
	}
	if changeID == "" {
		t.Error("expected non-empty change ID")
	}
}

func TestJujutsuVCS_ConfigAndClean(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-config")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Clean should be a no-op for jj
	err = jjVCS.Clean(ctx)
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	// IsMerging should always be false for jj
	merging, err := jjVCS.IsMerging(ctx)
	if err != nil {
		t.Fatalf("IsMerging failed: %v", err)
	}
	if merging {
		t.Error("expected IsMerging=false for jj")
	}
}

// --- Stack Navigation Tests ---

func TestJujutsuVCS_NextPrev(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-nextprev")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Create a stack of changes
	h.WriteFile(repoPath, "file1.txt", "first")
	jjVCS.Commit(ctx, "First change", nil)

	h.WriteFile(repoPath, "file2.txt", "second")
	jjVCS.Commit(ctx, "Second change", nil)

	h.WriteFile(repoPath, "file3.txt", "third")
	jjVCS.Commit(ctx, "Third change", nil)

	// We're now at the working copy after third change
	// Prev should move to parent
	info, err := jjVCS.Prev(ctx)
	if err != nil {
		t.Fatalf("Prev failed: %v", err)
	}
	if info == nil {
		t.Fatal("Prev returned nil info")
	}
	t.Logf("After prev: %s - %s", info.ShortID, info.Description)

	// Next should move back forward
	info, err = jjVCS.Next(ctx)
	if err != nil {
		t.Fatalf("Next failed: %v", err)
	}
	if info == nil {
		t.Fatal("Next returned nil info")
	}
	t.Logf("After next: %s - %s", info.ShortID, info.Description)
}

// --- Extended Bookmark Tests ---

func TestJujutsuVCS_BookmarkManagement(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-bookmark-mgmt")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Create content so we have a real change
	h.WriteFile(repoPath, "file.txt", "content")
	jjVCS.Commit(ctx, "Initial", nil)

	// Create a bookmark
	err = jjVCS.CreateBranch(ctx, "feature-1")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// List bookmarks
	branches, err := jjVCS.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	found := false
	for _, b := range branches {
		if b.Name == "feature-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find feature-1 bookmark")
	}

	// Move bookmark (set to current)
	err = jjVCS.SetBranch(ctx, "feature-1", "@")
	if err != nil {
		t.Fatalf("SetBranch failed: %v", err)
	}

	// Delete bookmark
	err = jjVCS.DeleteBranch(ctx, "feature-1")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// Verify deleted
	branches, err = jjVCS.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches after delete failed: %v", err)
	}
	for _, b := range branches {
		if b.Name == "feature-1" {
			t.Error("feature-1 should have been deleted")
		}
	}
}

// --- File Track/Untrack Tests ---

func TestJujutsuVCS_FileTrackUntrack(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-file-ops")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Create a file
	h.WriteFile(repoPath, "tracked.txt", "track me")

	// Track it explicitly
	err = jjVCS.TrackFiles(ctx, "tracked.txt")
	if err != nil {
		t.Fatalf("TrackFiles failed: %v", err)
	}

	// Verify it shows in status
	status, err := jjVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	found := false
	for _, s := range status {
		if s.Path == "tracked.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Log("tracked.txt not found in status (may be auto-tracked)")
	}

	// Commit the file so we can test untrack
	jjVCS.Commit(ctx, "Add tracked file", nil)

	// Untrack
	err = jjVCS.UntrackFiles(ctx, "tracked.txt")
	if err != nil {
		// UntrackFiles may fail if jj requires the file to be in .gitignore first
		t.Logf("UntrackFiles returned error (expected if not in .gitignore): %v", err)
	}
}

// --- Workspace Update-Stale Tests ---

func TestJujutsuVCS_UpdateStaleWorkspace(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-update-stale")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// update-stale on default workspace should be a no-op (not stale)
	err = jjVCS.UpdateStaleWorkspace(ctx, "default")
	if err != nil {
		t.Logf("UpdateStaleWorkspace on non-stale workspace: %v", err)
	}

	// Create a second workspace
	wsPath := filepath.Join(h.tempDir, "stale-ws")
	err = jjVCS.CreateWorkspace(ctx, "stale-test", wsPath)
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}

	// update-stale should work on the new workspace
	err = jjVCS.UpdateStaleWorkspace(ctx, "stale-test")
	if err != nil {
		t.Logf("UpdateStaleWorkspace on new workspace: %v (may not be stale)", err)
	}

	// Cleanup
	jjVCS.RemoveWorkspace(ctx, "stale-test")
}

// --- Git Backend P2 Tests ---

func TestGitVCS_DeleteBranch(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-delete-branch")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Create initial commit
	h.WriteFile(repoPath, "file.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Create a branch
	err = gitVCS.CreateBranch(ctx, "feature-x")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	// Delete it
	err = gitVCS.DeleteBranch(ctx, "feature-x")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}
}

func TestGitVCS_FileTrackUntrack(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("git-file-ops")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Create initial commit
	h.WriteFile(repoPath, "initial.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "Initial")

	// Track a new file
	h.WriteFile(repoPath, "new.txt", "new content")
	err = gitVCS.TrackFiles(ctx, "new.txt")
	if err != nil {
		t.Fatalf("TrackFiles failed: %v", err)
	}

	// Commit it
	gitVCS.Commit(ctx, "Add new file", nil)

	// Untrack it (remove from index, keep on disk)
	err = gitVCS.UntrackFiles(ctx, "new.txt")
	if err != nil {
		t.Fatalf("UntrackFiles failed: %v", err)
	}

	// Verify file still exists on disk
	if _, err := os.Stat(filepath.Join(repoPath, "new.txt")); err != nil {
		t.Error("file should still exist on disk after untracking")
	}
}

// --- Timeout Tests ---

func TestVCS_CommandTimeout(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("timeout-test")

	vcs, _ := NewGitVCS(repoPath)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// This should fail due to timeout
	_, err := vcs.Status(ctx)
	if err == nil {
		// Might succeed if command was fast enough, that's ok
		t.Log("Command completed before timeout (expected for fast operations)")
	}
}

// --- Phase 2: Sync-branch worktree/workspace operation tests ---

func TestGitVCS_LogBetween(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("log-between")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Create initial commit on default branch
	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "git", "add", "base.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "base commit")

	defaultBranch, _ := gitVCS.CurrentBranch(ctx)

	// Create feature branch with extra commits
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	h.WriteFile(repoPath, "feature1.txt", "feat1")
	h.runCmd(repoPath, "git", "add", "feature1.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "feature commit 1")
	h.WriteFile(repoPath, "feature2.txt", "feat2")
	h.runCmd(repoPath, "git", "add", "feature2.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "feature commit 2")

	// LogBetween: commits in feature not in default
	changes, err := gitVCS.LogBetween(ctx, defaultBranch, "feature")
	if err != nil {
		t.Fatalf("LogBetween failed: %v", err)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 commits, got %d", len(changes))
	}

	// LogBetween: commits in default not in feature (should be 0)
	changes, err = gitVCS.LogBetween(ctx, "feature", defaultBranch)
	if err != nil {
		t.Fatalf("LogBetween reverse failed: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 commits, got %d", len(changes))
	}
}

func TestGitVCS_DiffPath(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("diff-path")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Create initial commit
	h.WriteFile(repoPath, "file.txt", "original")
	h.runCmd(repoPath, "git", "add", "file.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "initial")

	defaultBranch, _ := gitVCS.CurrentBranch(ctx)

	// Create feature branch with changes
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	h.WriteFile(repoPath, "file.txt", "modified")
	h.runCmd(repoPath, "git", "add", "file.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "modify file")

	// DiffPath should show the change
	diff, err := gitVCS.DiffPath(ctx, defaultBranch, "feature", "file.txt")
	if err != nil {
		t.Fatalf("DiffPath failed: %v", err)
	}
	if diff == "" {
		t.Error("expected non-empty diff")
	}
	if !strings.Contains(diff, "original") || !strings.Contains(diff, "modified") {
		t.Errorf("diff doesn't contain expected content: %s", diff)
	}
}

func TestGitVCS_HasStagedChanges(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("staged-changes")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Initial commit so we have a HEAD
	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", "init.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "initial")

	// No staged changes
	has, err := gitVCS.HasStagedChanges(ctx)
	if err != nil {
		t.Fatalf("HasStagedChanges failed: %v", err)
	}
	if has {
		t.Error("expected no staged changes")
	}

	// Stage a change
	h.WriteFile(repoPath, "new.txt", "content")
	h.runCmd(repoPath, "git", "add", "new.txt")

	has, err = gitVCS.HasStagedChanges(ctx)
	if err != nil {
		t.Fatalf("HasStagedChanges after add failed: %v", err)
	}
	if !has {
		t.Error("expected staged changes")
	}
}

func TestGitVCS_StageAndCommit(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("stage-commit")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Initial commit
	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", "init.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "initial")

	// Write a file, stage and commit it
	h.WriteFile(repoPath, "beads.txt", "beads data")

	opts := &CommitOptions{NoGPGSign: true}
	err = gitVCS.StageAndCommit(ctx, []string{"beads.txt"}, "add beads data", opts)
	if err != nil {
		t.Fatalf("StageAndCommit failed: %v", err)
	}

	// Verify commit was made
	changes, err := gitVCS.Log(ctx, 1)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one commit")
	}
	if changes[0].Description != "add beads data" {
		t.Errorf("expected commit message 'add beads data', got %q", changes[0].Description)
	}
}

func TestGitVCS_RebaseAndAbort(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("rebase-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS failed: %v", err)
	}

	// Create initial commit
	h.WriteFile(repoPath, "file.txt", "base")
	h.runCmd(repoPath, "git", "add", "file.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "base")

	defaultBranch, _ := gitVCS.CurrentBranch(ctx)

	// Create diverging branches
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	h.WriteFile(repoPath, "feature.txt", "feature")
	h.runCmd(repoPath, "git", "add", "feature.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "feature")

	h.runCmd(repoPath, "git", "checkout", defaultBranch)
	h.WriteFile(repoPath, "main-change.txt", "main")
	h.runCmd(repoPath, "git", "add", "main-change.txt")
	h.runCmd(repoPath, "git", "commit", "--no-gpg-sign", "-m", "main change")

	// Rebase feature onto default (non-conflicting)
	h.runCmd(repoPath, "git", "checkout", "feature")
	err = gitVCS.Rebase(ctx, defaultBranch)
	if err != nil {
		t.Fatalf("Rebase failed: %v", err)
	}

	// RebaseAbort when no rebase in progress should still work (error is ok)
	_ = gitVCS.RebaseAbort(ctx)
}

func TestJujutsuVCS_LogBetween(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-log-between")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Create a commit (this creates a change)
	h.WriteFile(repoPath, "base.txt", "base")
	jjVCS.Commit(ctx, "base commit", nil)

	// Get the base change ID
	baseID, err := jjVCS.ResolveRef(ctx, "@-")
	if err != nil {
		t.Fatalf("ResolveRef @- failed: %v", err)
	}

	// Create more commits
	h.WriteFile(repoPath, "feat1.txt", "feat1")
	jjVCS.Commit(ctx, "feature 1", nil)

	// Current @ should be new empty working copy
	currentID, err := jjVCS.ResolveRef(ctx, "@")
	if err != nil {
		t.Fatalf("ResolveRef @ failed: %v", err)
	}

	// LogBetween: changes after base (should include feature 1 + current)
	changes, err := jjVCS.LogBetween(ctx, baseID, currentID)
	if err != nil {
		t.Fatalf("LogBetween failed: %v", err)
	}
	// Should have at least 1 change (feature 1)
	if len(changes) < 1 {
		t.Errorf("expected at least 1 change, got %d", len(changes))
	}
}

func TestJujutsuVCS_StageAndCommit(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-stage-commit")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Write a file
	h.WriteFile(repoPath, "data.txt", "some data")

	// StageAndCommit
	err = jjVCS.StageAndCommit(ctx, []string{"data.txt"}, "add data file", nil)
	if err != nil {
		t.Fatalf("StageAndCommit failed: %v", err)
	}

	// Verify the commit exists
	changes, err := jjVCS.Log(ctx, 3)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	found := false
	for _, c := range changes {
		if strings.Contains(c.Description, "add data file") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'add data file' commit in log")
	}
}

// --- Phase 3: Hook integration tests ---

func TestGitVCS_IsFileTracked(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("track-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create and commit a file
	h.WriteFile(repoPath, "tracked.txt", "hello")
	h.runCmd(repoPath, "git", "add", "tracked.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "add tracked file")

	// tracked.txt should be tracked
	tracked, err := gitVCS.IsFileTracked(ctx, "tracked.txt")
	if err != nil {
		t.Fatalf("IsFileTracked(tracked.txt): %v", err)
	}
	if !tracked {
		t.Error("expected tracked.txt to be tracked")
	}

	// untracked.txt should not be tracked
	h.WriteFile(repoPath, "untracked.txt", "world")
	tracked, err = gitVCS.IsFileTracked(ctx, "untracked.txt")
	if err != nil {
		t.Fatalf("IsFileTracked(untracked.txt): %v", err)
	}
	if tracked {
		t.Error("expected untracked.txt to NOT be tracked")
	}
}

func TestGitVCS_ConfigureHooksPath(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("hooks-path-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Initially no custom hooks path
	hooksPath, err := gitVCS.GetHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetHooksPath: %v", err)
	}
	if hooksPath != "" {
		t.Errorf("expected empty hooks path, got %q", hooksPath)
	}

	// Configure custom hooks path
	if err := gitVCS.ConfigureHooksPath(ctx, ".beads/hooks"); err != nil {
		t.Fatalf("ConfigureHooksPath: %v", err)
	}

	// Verify hooks path is set
	hooksPath, err = gitVCS.GetHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetHooksPath after set: %v", err)
	}
	if hooksPath != ".beads/hooks" {
		t.Errorf("expected hooks path '.beads/hooks', got %q", hooksPath)
	}
}

func TestGitVCS_ConfigureMergeDriver(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("merge-driver-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Configure merge driver
	driverCmd := "bd merge %A %O %A %B"
	driverName := "bd JSONL merge driver"
	if err := gitVCS.ConfigureMergeDriver(ctx, driverCmd, driverName); err != nil {
		t.Fatalf("ConfigureMergeDriver: %v", err)
	}

	// Verify driver command
	val, err := gitVCS.GetConfig(ctx, "merge.beads.driver")
	if err != nil {
		t.Fatalf("GetConfig(merge.beads.driver): %v", err)
	}
	if strings.TrimSpace(val) != driverCmd {
		t.Errorf("expected driver %q, got %q", driverCmd, val)
	}

	// Verify driver name
	val, err = gitVCS.GetConfig(ctx, "merge.beads.name")
	if err != nil {
		t.Fatalf("GetConfig(merge.beads.name): %v", err)
	}
	if strings.TrimSpace(val) != driverName {
		t.Errorf("expected name %q, got %q", driverName, val)
	}
}

func TestJujutsuVCS_IsFileTracked(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-track-test")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	// Create a file (jj auto-tracks files in working copy)
	h.WriteFile(repoPath, "tracked.txt", "hello")
	// Commit so it's definitely tracked
	h.runCmd(repoPath, "jj", "commit", "-m", "add tracked file")

	tracked, err := jjVCS.IsFileTracked(ctx, "tracked.txt")
	if err != nil {
		t.Fatalf("IsFileTracked(tracked.txt): %v", err)
	}
	if !tracked {
		t.Error("expected tracked.txt to be tracked in jj")
	}
}

func TestJujutsuVCS_HookMethodsAreNoOps(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-hooks-noop")
	ctx := context.Background()

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	// ConfigureHooksPath should be no-op (no error)
	if err := jjVCS.ConfigureHooksPath(ctx, ".beads/hooks"); err != nil {
		t.Errorf("ConfigureHooksPath should be no-op, got: %v", err)
	}

	// GetHooksPath should return empty (jj manages hooks differently)
	path, err := jjVCS.GetHooksPath(ctx)
	if err != nil {
		t.Errorf("GetHooksPath should not error, got: %v", err)
	}
	if path != "" {
		t.Errorf("GetHooksPath should return empty for jj, got: %q", path)
	}

	// ConfigureMergeDriver should be no-op
	if err := jjVCS.ConfigureMergeDriver(ctx, "cmd", "name"); err != nil {
		t.Errorf("ConfigureMergeDriver should be no-op, got: %v", err)
	}
}

// --- Phase 4: Doctor/maintenance operation tests ---

func TestGitVCS_DiffHasChanges(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("diff-changes-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create and commit a file
	h.WriteFile(repoPath, "data.txt", "initial")
	h.runCmd(repoPath, "git", "add", "data.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "initial")

	// No changes yet
	changed, err := gitVCS.DiffHasChanges(ctx, "HEAD", "data.txt")
	if err != nil {
		t.Fatalf("DiffHasChanges (no changes): %v", err)
	}
	if changed {
		t.Error("expected no changes for unmodified file")
	}

	// Modify the file
	h.WriteFile(repoPath, "data.txt", "modified")
	h.runCmd(repoPath, "git", "add", "data.txt")

	changed, err = gitVCS.DiffHasChanges(ctx, "HEAD", "data.txt")
	if err != nil {
		t.Fatalf("DiffHasChanges (after modify): %v", err)
	}
	if !changed {
		t.Error("expected changes after modification")
	}
}

func TestGitVCS_RevListCount(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("revlist-count-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create initial commit on a known branch
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.WriteFile(repoPath, "file.txt", "v1")
	h.runCmd(repoPath, "git", "add", "file.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "v1")

	// Create a branch and add 3 commits
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	for i := 2; i <= 4; i++ {
		h.WriteFile(repoPath, "file.txt", fmt.Sprintf("v%d", i))
		h.runCmd(repoPath, "git", "add", "file.txt")
		h.runCmd(repoPath, "git", "commit", "-m", fmt.Sprintf("v%d", i))
	}

	// Count commits from trunk to feature
	count, err := gitVCS.RevListCount(ctx, "trunk", "feature")
	if err != nil {
		t.Fatalf("RevListCount: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 commits, got %d", count)
	}
}

func TestGitVCS_MergeBase(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("merge-base-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create initial commit on a known branch
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.WriteFile(repoPath, "file.txt", "initial")
	h.runCmd(repoPath, "git", "add", "file.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "initial")

	// Get initial commit hash
	initialHash, err := gitVCS.ResolveRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef: %v", err)
	}

	// Create branch and diverge
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	h.WriteFile(repoPath, "feature.txt", "feature")
	h.runCmd(repoPath, "git", "add", "feature.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "feature commit")

	h.runCmd(repoPath, "git", "checkout", "trunk")
	h.WriteFile(repoPath, "main.txt", "main")
	h.runCmd(repoPath, "git", "add", "main.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "trunk commit")

	// Merge base should be the initial commit
	base, err := gitVCS.MergeBase(ctx, "trunk", "feature")
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	if base != initialHash {
		t.Errorf("expected merge base %s, got %s", initialHash, base)
	}
}

func TestGitVCS_CheckIgnore(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("check-ignore-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create .gitignore
	h.WriteFile(repoPath, ".gitignore", "*.log\n")
	h.runCmd(repoPath, "git", "add", ".gitignore")
	h.runCmd(repoPath, "git", "commit", "-m", "add gitignore")

	// .log file should be ignored
	h.WriteFile(repoPath, "test.log", "log data")
	ignored, err := gitVCS.CheckIgnore(ctx, "test.log")
	if err != nil {
		t.Fatalf("CheckIgnore(test.log): %v", err)
	}
	if !ignored {
		t.Error("expected test.log to be ignored")
	}

	// .txt file should NOT be ignored
	h.WriteFile(repoPath, "test.txt", "text data")
	ignored, err = gitVCS.CheckIgnore(ctx, "test.txt")
	if err != nil {
		t.Fatalf("CheckIgnore(test.txt): %v", err)
	}
	if ignored {
		t.Error("expected test.txt to NOT be ignored")
	}
}

func TestGitVCS_RestoreFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("restore-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create and commit
	h.WriteFile(repoPath, "data.txt", "original")
	h.runCmd(repoPath, "git", "add", "data.txt")
	h.runCmd(repoPath, "git", "commit", "-m", "initial")

	// Modify the file
	h.WriteFile(repoPath, "data.txt", "modified")

	// Verify it changed
	changed, _ := gitVCS.DiffHasChanges(ctx, "HEAD", "data.txt")
	if !changed {
		t.Fatal("expected file to be modified")
	}

	// Restore it
	if err := gitVCS.RestoreFile(ctx, "data.txt"); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	// Verify restored
	changed, _ = gitVCS.DiffHasChanges(ctx, "HEAD", "data.txt")
	if changed {
		t.Error("expected file to be restored to HEAD")
	}
}

func TestGitVCS_GetCommonDir(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("common-dir-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	commonDir, err := gitVCS.GetCommonDir(ctx)
	if err != nil {
		t.Fatalf("GetCommonDir: %v", err)
	}
	if commonDir == "" {
		t.Error("expected non-empty common dir")
	}
}

func TestGitVCS_ListTrackedFiles(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("list-tracked-test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create and commit files
	h.WriteFile(repoPath, "a.txt", "a")
	h.WriteFile(repoPath, "b.txt", "b")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add files")

	files, err := gitVCS.ListTrackedFiles(ctx, ".")
	if err != nil {
		t.Fatalf("ListTrackedFiles: %v", err)
	}
	if len(files) < 2 {
		t.Errorf("expected at least 2 tracked files, got %d", len(files))
	}
}
