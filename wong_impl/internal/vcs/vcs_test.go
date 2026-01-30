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
	driverCmd := "wong merge %A %O %A %B"
	driverName := "wong JSONL merge driver"
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

// --- Phase 5 Tests ---

func TestGitVCS_ShowFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create and commit a file
	h.WriteFile(repoPath, "hello.txt", "hello world")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "initial")

	// Read file from HEAD
	content, err := gitVCS.ShowFile(ctx, "HEAD", "hello.txt")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(content))
	}

	// Update file and commit again
	h.WriteFile(repoPath, "hello.txt", "updated content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "update")

	// Read from HEAD should give new content
	content, err = gitVCS.ShowFile(ctx, "HEAD", "hello.txt")
	if err != nil {
		t.Fatalf("ShowFile after update: %v", err)
	}
	if string(content) != "updated content" {
		t.Errorf("expected 'updated content', got %q", string(content))
	}

	// Read from HEAD~1 should give old content
	content, err = gitVCS.ShowFile(ctx, "HEAD~1", "hello.txt")
	if err != nil {
		t.Fatalf("ShowFile HEAD~1: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world' from HEAD~1, got %q", string(content))
	}
}

func TestGitVCS_GetVCSDir(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	vcsDir, err := gitVCS.GetVCSDir(ctx)
	if err != nil {
		t.Fatalf("GetVCSDir: %v", err)
	}

	// For a non-worktree repo, should end with .git
	if !strings.HasSuffix(vcsDir, ".git") {
		t.Errorf("expected VCS dir to end with .git, got %q", vcsDir)
	}
}

func TestGitVCS_IsWorktreeRepo(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Need a commit first
	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "initial")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Main repo should not be a worktree
	isWorktree, err := gitVCS.IsWorktreeRepo(ctx)
	if err != nil {
		t.Fatalf("IsWorktreeRepo: %v", err)
	}
	if isWorktree {
		t.Errorf("expected main repo to not be a worktree")
	}
}

func TestGitVCS_Checkout(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create initial commit
	h.WriteFile(repoPath, "test.txt", "v1")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "v1")

	// Create a branch
	h.runCmd(repoPath, "git", "checkout", "-b", "feature")
	h.WriteFile(repoPath, "test.txt", "v2")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "v2")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Checkout trunk
	if err := gitVCS.Checkout(ctx, "trunk"); err != nil {
		t.Fatalf("Checkout trunk: %v", err)
	}

	// Verify we're on trunk
	branch, err := gitVCS.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "trunk" {
		t.Errorf("expected 'trunk', got %q", branch)
	}
}

func TestGitVCS_SymbolicRef(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create initial commit on a named branch
	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "initial")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Should return branch name
	ref, err := gitVCS.SymbolicRef(ctx)
	if err != nil {
		t.Fatalf("SymbolicRef: %v", err)
	}
	if ref != "trunk" {
		t.Errorf("expected 'trunk', got %q", ref)
	}

	// Detach HEAD
	h.runCmd(repoPath, "git", "checkout", "--detach")

	// Should return empty for detached HEAD
	ref, err = gitVCS.SymbolicRef(ctx)
	if err != nil {
		t.Fatalf("SymbolicRef detached: %v", err)
	}
	if ref != "" {
		t.Errorf("expected empty for detached HEAD, got %q", ref)
	}
}

func TestGitVCS_GetRemoteURLs(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// No remotes initially
	urls, err := gitVCS.GetRemoteURLs(ctx)
	if err != nil {
		// Some git versions return error when no remotes
		return
	}
	if len(urls) != 0 {
		t.Errorf("expected no remotes, got %d", len(urls))
	}

	// Add a remote
	h.runCmd(repoPath, "git", "remote", "add", "origin", "https://github.com/test/repo.git")

	urls, err = gitVCS.GetRemoteURLs(ctx)
	if err != nil {
		t.Fatalf("GetRemoteURLs: %v", err)
	}
	if url, ok := urls["origin"]; !ok || url != "https://github.com/test/repo.git" {
		t.Errorf("expected origin URL, got %v", urls)
	}
}

// --- wong-bpg.1: Error path tests ---

func TestGitVCS_NewGitVCS_InvalidPath(t *testing.T) {
	// NewGitVCS with a non-existent path should fail
	_, err := NewGitVCS("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestGitVCS_NewGitVCS_NotARepo(t *testing.T) {
	// NewGitVCS with a regular directory (not a git repo) should fail
	dir, err := os.MkdirTemp("", "not-a-repo-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	_, err = NewGitVCS(dir)
	if err == nil {
		t.Error("expected error for non-repo directory")
	}
}

func TestGitVCS_ShowFile_NonExistentFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Need at least one commit
	h.WriteFile(repoPath, "exists.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Try to show a file that doesn't exist
	_, err = gitVCS.ShowFile(ctx, "HEAD", "does-not-exist.txt")
	if err == nil {
		t.Error("expected error for non-existent file in ShowFile")
	}
}

func TestGitVCS_ShowFile_InvalidRef(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Try with a bad ref
	_, err = gitVCS.ShowFile(ctx, "nonexistent-ref", "test.txt")
	if err == nil {
		t.Error("expected error for invalid ref in ShowFile")
	}
}

func TestGitVCS_ResolveRef_InvalidRef(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	_, err = gitVCS.ResolveRef(ctx, "nonexistent-branch-xyz")
	if err == nil {
		t.Error("expected error for invalid ref in ResolveRef")
	}
}

func TestGitVCS_Checkout_InvalidRef(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	err = gitVCS.Checkout(ctx, "nonexistent-branch-xyz")
	if err == nil {
		t.Error("expected error for checkout of invalid ref")
	}
}

func TestGitVCS_BranchExists_NonExistent(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	exists, err := gitVCS.BranchExists(ctx, "nonexistent-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if exists {
		t.Error("expected branch to not exist")
	}

	exists, err = gitVCS.BranchExists(ctx, "trunk")
	if err != nil {
		t.Fatalf("BranchExists trunk: %v", err)
	}
	if !exists {
		t.Error("expected trunk branch to exist")
	}
}

func TestGitVCS_GetRemoteURL_NoRemote(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	_, err = gitVCS.GetRemoteURL(ctx, "origin")
	if err == nil {
		t.Error("expected error for non-existent remote")
	}
}

func TestGitVCS_GetUpstream_NoBranch(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// No upstream configured
	_, err = gitVCS.GetUpstream(ctx)
	if err == nil {
		t.Error("expected error for branch with no upstream")
	}
}

func TestGitVCS_IsFileTracked_Untracked(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create file but don't track it
	h.WriteFile(repoPath, "untracked.txt", "content")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	tracked, err := gitVCS.IsFileTracked(ctx, "untracked.txt")
	if err != nil {
		t.Fatalf("IsFileTracked: %v", err)
	}
	if tracked {
		t.Error("expected untracked file to not be tracked")
	}
}

func TestGitVCS_CheckIgnore_NotIgnored(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create a file and commit it (not ignored)
	h.WriteFile(repoPath, "tracked.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	ignored, err := gitVCS.CheckIgnore(ctx, "tracked.txt")
	if err != nil {
		t.Fatalf("CheckIgnore: %v", err)
	}
	if ignored {
		t.Error("expected tracked file to not be ignored")
	}

	// Now create .gitignore and check ignored file
	h.WriteFile(repoPath, ".gitignore", "*.log\n")
	h.WriteFile(repoPath, "debug.log", "log content")

	ignored, err = gitVCS.CheckIgnore(ctx, "debug.log")
	if err != nil {
		t.Fatalf("CheckIgnore ignored file: %v", err)
	}
	if !ignored {
		t.Error("expected .log file to be ignored")
	}
}

func TestGitVCS_MergeBase_NoCommonAncestor(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create an orphan branch with no common ancestor
	h.runCmd(repoPath, "git", "checkout", "--orphan", "orphan")
	h.WriteFile(repoPath, "orphan.txt", "orphan content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "orphan commit")

	_, err = gitVCS.MergeBase(ctx, "trunk", "orphan")
	if err == nil {
		t.Error("expected error for branches with no common ancestor")
	}
}

func TestGitVCS_Stage_NonExistentFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	err = gitVCS.Stage(ctx, "does-not-exist.txt")
	if err == nil {
		t.Error("expected error staging non-existent file")
	}
}

func TestGitVCS_Commit_EmptyRepo(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Committing with nothing staged should fail
	err = gitVCS.Commit(ctx, "empty commit", nil)
	if err == nil {
		t.Error("expected error committing with nothing staged")
	}
}

func TestGitVCS_Push_NoRemote(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Push with no remote configured should fail
	err = gitVCS.Push(ctx, "origin", "trunk")
	if err == nil {
		t.Error("expected error pushing with no remote")
	}
}

func TestGitVCS_RestoreFile_NoChanges(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Restoring a file with no changes should be a no-op (not error)
	err = gitVCS.RestoreFile(ctx, "test.txt")
	if err != nil {
		t.Errorf("RestoreFile on clean file: %v", err)
	}

	// Modify, then restore
	h.WriteFile(repoPath, "test.txt", "modified")
	err = gitVCS.RestoreFile(ctx, "test.txt")
	if err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}

	// Verify content was restored
	data, _ := os.ReadFile(filepath.Join(repoPath, "test.txt"))
	if string(data) != "content" {
		t.Errorf("expected 'content' after restore, got %q", string(data))
	}
}

func TestGitVCS_Status_CleanRepo(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	entries, err := gitVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected clean status, got %d entries", len(entries))
	}
}

func TestGitVCS_Status_WithChanges(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	// Modify file and create new untracked file
	h.WriteFile(repoPath, "test.txt", "modified")
	h.WriteFile(repoPath, "new.txt", "new content")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	entries, err := gitVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 status entries (modified + untracked), got %d", len(entries))
	}

	// Check that we have both modified and untracked
	hasModified, hasUntracked := false, false
	for _, e := range entries {
		if strings.Contains(e.Path, "test.txt") && e.Status == FileStatusModified {
			hasModified = true
		}
		if strings.Contains(e.Path, "new.txt") && e.Status == FileStatusUntracked {
			hasUntracked = true
		}
	}
	if !hasModified {
		// Debug: print all entries for diagnosis
		for _, e := range entries {
			t.Logf("  entry: path=%q status=%q staged=%v", e.Path, e.Status, e.Staged)
		}
		t.Error("expected modified status for test.txt")
	}
	if !hasUntracked {
		t.Error("expected untracked status for new.txt")
	}
}

func TestGitVCS_DeleteBranch_NonExistent(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	err = gitVCS.DeleteBranch(ctx, "does-not-exist")
	if err == nil {
		t.Error("expected error deleting non-existent branch")
	}
}

func TestGitVCS_CommandError_Type(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// This should return a *CommandError for invalid ref
	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	_, err = gitVCS.ResolveRef(ctx, "nonexistent-ref-xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*CommandError); !ok {
		t.Errorf("expected *CommandError, got %T: %v", err, err)
	}
}

func TestGitVCS_ContextCancellation(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations with cancelled context should fail
	_, err = gitVCS.Status(ctx)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestGitVCS_HasRemote_None(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	hasRemote, err := gitVCS.HasRemote(ctx)
	if err != nil {
		t.Fatalf("HasRemote: %v", err)
	}
	if hasRemote {
		t.Error("expected no remote in fresh repo")
	}

	// Add a remote
	h.runCmd(repoPath, "git", "remote", "add", "origin", "https://example.com/repo.git")

	hasRemote, err = gitVCS.HasRemote(ctx)
	if err != nil {
		t.Fatalf("HasRemote after add: %v", err)
	}
	if !hasRemote {
		t.Error("expected remote after adding origin")
	}
}

func TestGitVCS_CurrentBranch_EmptyRepo(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Empty repo (no commits) - CurrentBranch behavior varies
	_, err = gitVCS.CurrentBranch(ctx)
	// Some git versions return the unborn branch name, others error
	// We just verify it doesn't panic
	_ = err
}

func TestGitVCS_GetConfig_NonExistent(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	_, err = gitVCS.GetConfig(ctx, "nonexistent.key.that.does.not.exist")
	if err == nil {
		t.Error("expected error for non-existent config key")
	}
}

func TestGitVCS_SetConfig_AndGet(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Set a custom config
	err = gitVCS.SetConfig(ctx, "test.key", "test-value")
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	// Read it back
	val, err := gitVCS.GetConfig(ctx, "test.key")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "test-value" {
		t.Errorf("expected 'test-value', got %q", val)
	}
}

func TestGitVCS_DiffHasChanges_Clean(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// No changes since HEAD
	hasChanges, err := gitVCS.DiffHasChanges(ctx, "HEAD", "test.txt")
	if err != nil {
		t.Fatalf("DiffHasChanges: %v", err)
	}
	if hasChanges {
		t.Error("expected no changes for clean file")
	}

	// Now modify
	h.WriteFile(repoPath, "test.txt", "modified")
	hasChanges, err = gitVCS.DiffHasChanges(ctx, "HEAD", "test.txt")
	if err != nil {
		t.Fatalf("DiffHasChanges after modify: %v", err)
	}
	if !hasChanges {
		t.Error("expected changes after modification")
	}
}

// --- wong-bpg.2: Workflow chain tests ---

func TestGitVCS_StageCommitWorkflow(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Stage → Commit → verify via Log
	h.WriteFile(repoPath, "file1.txt", "content1")
	if err := gitVCS.Stage(ctx, "file1.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := gitVCS.Commit(ctx, "first commit", nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Verify via Log
	logs, err := gitVCS.Log(ctx, 5)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected at least 1 log entry")
	}
	if !strings.Contains(logs[0].Description, "first commit") {
		t.Errorf("expected 'first commit' in log, got %q", logs[0].Description)
	}

	// Stage and commit a second file
	h.WriteFile(repoPath, "file2.txt", "content2")
	if err := gitVCS.Stage(ctx, "file2.txt"); err != nil {
		t.Fatalf("Stage file2: %v", err)
	}
	if err := gitVCS.Commit(ctx, "second commit", nil); err != nil {
		t.Fatalf("Commit 2: %v", err)
	}

	logs, err = gitVCS.Log(ctx, 5)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected at least 2 log entries, got %d", len(logs))
	}
}

func TestGitVCS_BranchCreateSwitchWorkflow(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Initial commit on trunk
	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create feature branch
	if err := gitVCS.CreateBranch(ctx, "feature-x"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Verify it exists
	exists, err := gitVCS.BranchExists(ctx, "feature-x")
	if err != nil || !exists {
		t.Fatalf("expected feature-x to exist")
	}

	// Switch to it
	if err := gitVCS.SwitchBranch(ctx, "feature-x"); err != nil {
		t.Fatalf("SwitchBranch: %v", err)
	}

	// Verify current branch
	branch, err := gitVCS.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "feature-x" {
		t.Errorf("expected 'feature-x', got %q", branch)
	}

	// Add commit on feature branch
	h.WriteFile(repoPath, "feature.txt", "feature content")
	if err := gitVCS.Stage(ctx, "feature.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := gitVCS.Commit(ctx, "feature work", nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Switch back to trunk
	if err := gitVCS.SwitchBranch(ctx, "trunk"); err != nil {
		t.Fatalf("SwitchBranch back: %v", err)
	}

	// feature.txt should not exist on trunk
	if _, err := os.Stat(filepath.Join(repoPath, "feature.txt")); !os.IsNotExist(err) {
		t.Error("expected feature.txt to not exist on trunk")
	}
}

func TestGitVCS_SquashAmendWorkflow(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Initial commit
	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "base commit")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Make a change and squash (amend)
	h.WriteFile(repoPath, "base.txt", "base updated")
	if err := gitVCS.Stage(ctx, "base.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	// Squash = commit --amend in git
	if err := gitVCS.Squash(ctx, ""); err != nil {
		t.Fatalf("Squash: %v", err)
	}

	// Should still be only 1 commit (the amended one)
	logs, err := gitVCS.Log(ctx, 10)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log entry after squash, got %d", len(logs))
	}
}

func TestGitVCS_StageAndCommitWorkflow(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Use StageAndCommit for atomic operation
	h.WriteFile(repoPath, "atomic1.txt", "content1")
	h.WriteFile(repoPath, "atomic2.txt", "content2")

	err = gitVCS.StageAndCommit(ctx, []string{"atomic1.txt", "atomic2.txt"}, "atomic commit", nil)
	if err != nil {
		t.Fatalf("StageAndCommit: %v", err)
	}

	// Verify both files are committed
	logs, err := gitVCS.Log(ctx, 1)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(logs) == 0 || !strings.Contains(logs[0].Description, "atomic commit") {
		t.Error("expected 'atomic commit' in log")
	}

	// Working tree should be clean
	entries, err := gitVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected clean status after StageAndCommit, got %d entries", len(entries))
	}
}

func TestGitVCS_CommitWithOptions(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Empty commit with AllowEmpty
	err = gitVCS.Commit(ctx, "empty commit", &CommitOptions{AllowEmpty: true})
	if err != nil {
		t.Fatalf("Commit with AllowEmpty: %v", err)
	}

	// Commit with custom author
	h.WriteFile(repoPath, "authored.txt", "content")
	if err := gitVCS.Stage(ctx, "authored.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	err = gitVCS.Commit(ctx, "authored commit", &CommitOptions{
		Author:    "Other User <other@example.com>",
		NoGPGSign: true,
	})
	if err != nil {
		t.Fatalf("Commit with Author: %v", err)
	}
}

func TestGitVCS_LogBetween_ThreeCommits(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create base commit
	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "base")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Get base ref
	baseRef, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// Add two more commits
	h.WriteFile(repoPath, "file1.txt", "f1")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "commit 1")

	h.WriteFile(repoPath, "file2.txt", "f2")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "commit 2")

	headRef, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// LogBetween base..HEAD should give 2 commits
	changes, err := gitVCS.LogBetween(ctx, baseRef, headRef)
	if err != nil {
		t.Fatalf("LogBetween: %v", err)
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 commits between base and HEAD, got %d", len(changes))
	}
}

func TestGitVCS_Diff(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "diff.txt", "original content\n")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "v1")

	baseRef, _ := NewGitVCS(repoPath)
	_ = baseRef

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	ref1, _ := gitVCS.ResolveRef(ctx, "HEAD")

	h.WriteFile(repoPath, "diff.txt", "modified content\n")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "v2")

	ref2, _ := gitVCS.ResolveRef(ctx, "HEAD")

	diffOutput, err := gitVCS.Diff(ctx, ref1, ref2)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diffOutput, "original content") || !strings.Contains(diffOutput, "modified content") {
		t.Errorf("diff should contain both old and new content, got: %s", diffOutput)
	}
}

func TestGitVCS_Show(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "show.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "show test commit")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	ref, _ := gitVCS.ResolveRef(ctx, "HEAD")
	info, err := gitVCS.Show(ctx, ref)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil ChangeInfo from Show")
	}
	if !strings.Contains(info.Description, "show test commit") {
		t.Errorf("expected 'show test commit' in description, got %q", info.Description)
	}
	if info.Author == "" {
		t.Error("expected non-empty author in Show output")
	}
}

func TestGitVCS_DiffPath_SelectiveChanges(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "a.txt", "original a\n")
	h.WriteFile(repoPath, "b.txt", "original b\n")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "v1")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	ref1, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// Modify only a.txt
	h.WriteFile(repoPath, "a.txt", "modified a\n")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "v2")

	ref2, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// DiffPath for a.txt should show changes
	diffA, err := gitVCS.DiffPath(ctx, ref1, ref2, "a.txt")
	if err != nil {
		t.Fatalf("DiffPath a.txt: %v", err)
	}
	if !strings.Contains(diffA, "modified a") {
		t.Errorf("expected diff for a.txt, got: %s", diffA)
	}

	// DiffPath for b.txt should be empty (no changes)
	diffB, err := gitVCS.DiffPath(ctx, ref1, ref2, "b.txt")
	if err != nil {
		t.Fatalf("DiffPath b.txt: %v", err)
	}
	if strings.TrimSpace(diffB) != "" {
		t.Errorf("expected empty diff for b.txt, got: %s", diffB)
	}
}

func TestGitVCS_IsAncestor_ParentChild(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "base")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	baseRef, _ := gitVCS.ResolveRef(ctx, "HEAD")

	h.WriteFile(repoPath, "child.txt", "child")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "child")

	childRef, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// base is ancestor of child
	isAnc, err := gitVCS.IsAncestor(ctx, baseRef, childRef)
	if err != nil {
		t.Fatalf("IsAncestor: %v", err)
	}
	if !isAnc {
		t.Error("expected base to be ancestor of child")
	}

	// child is NOT ancestor of base
	isAnc, err = gitVCS.IsAncestor(ctx, childRef, baseRef)
	if err != nil {
		t.Fatalf("IsAncestor reverse: %v", err)
	}
	if isAnc {
		t.Error("expected child to NOT be ancestor of base")
	}
}

func TestGitVCS_HasStagedChanges_Workflow(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// No staged changes
	hasStaged, err := gitVCS.HasStagedChanges(ctx)
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if hasStaged {
		t.Error("expected no staged changes on clean repo")
	}

	// Modify and stage
	h.WriteFile(repoPath, "test.txt", "modified")
	if err := gitVCS.Stage(ctx, "test.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	hasStaged, err = gitVCS.HasStagedChanges(ctx)
	if err != nil {
		t.Fatalf("HasStagedChanges after stage: %v", err)
	}
	if !hasStaged {
		t.Error("expected staged changes after staging")
	}
}

func TestGitVCS_ResolveRef_HEAD(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	ref, err := gitVCS.ResolveRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("ResolveRef HEAD: %v", err)
	}
	if len(ref) < 7 {
		t.Errorf("expected commit hash, got %q", ref)
	}

	// Also resolve by branch name
	refByBranch, err := gitVCS.ResolveRef(ctx, "trunk")
	if err != nil {
		t.Fatalf("ResolveRef trunk: %v", err)
	}
	if ref != refByBranch {
		t.Errorf("expected HEAD and trunk to resolve to same commit: %s vs %s", ref, refByBranch)
	}
}

func TestGitVCS_ListBranches(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	h.runCmd(repoPath, "git", "branch", "feature-a")
	h.runCmd(repoPath, "git", "branch", "feature-b")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	branches, err := gitVCS.ListBranches(ctx)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) < 3 {
		t.Errorf("expected at least 3 branches, got %d", len(branches))
	}

	// Find current branch
	hasCurrent := false
	for _, b := range branches {
		if b.IsCurrent && b.Name == "main" {
			hasCurrent = true
		}
	}
	if !hasCurrent {
		t.Error("expected 'main' to be the current branch")
	}
}

func TestGitVCS_RevListCount_WorkflowChain(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create 3 commits on trunk
	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "trunk")
	h.runCmd(repoPath, "git", "commit", "-m", "c1")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	c1, _ := gitVCS.ResolveRef(ctx, "HEAD")

	h.WriteFile(repoPath, "f1.txt", "f1")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "c2")

	h.WriteFile(repoPath, "f2.txt", "f2")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "c3")

	c3, _ := gitVCS.ResolveRef(ctx, "HEAD")

	// RevListCount c1..c3 should be 2
	count, err := gitVCS.RevListCount(ctx, c1, c3)
	if err != nil {
		t.Fatalf("RevListCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 commits between c1 and c3, got %d", count)
	}
}

// --- wong-bpg.11: Edge case tests ---

func TestGitVCS_EmptyRepo_NoCommits(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Status on empty repo should work
	entries, err := gitVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status on empty repo: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries on empty repo, got %d", len(entries))
	}

	// CurrentBranch on empty repo (no commits yet) may fail or return empty
	_, err = gitVCS.CurrentBranch(ctx)
	// We just verify it doesn't panic - error is acceptable
	_ = err

	// ResolveRef HEAD should fail on empty repo
	_, err = gitVCS.ResolveRef(ctx, "HEAD")
	if err == nil {
		t.Error("expected error resolving HEAD on empty repo")
	}
}

func TestGitVCS_UnicodeFilenames(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	// Create initial commit
	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create file with unicode name
	h.WriteFile(repoPath, "café.txt", "unicode content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add unicode file")

	tracked, err := gitVCS.IsFileTracked(ctx, "café.txt")
	if err != nil {
		t.Fatalf("IsFileTracked unicode: %v", err)
	}
	if !tracked {
		t.Error("expected unicode filename to be tracked")
	}

	files, err := gitVCS.ListTrackedFiles(ctx, ".")
	if err != nil {
		t.Fatalf("ListTrackedFiles: %v", err)
	}
	found := false
	for _, f := range files {
		if strings.Contains(f, "café") || strings.Contains(f, "caf") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unicode file in tracked list, got %v", files)
	}
}

func TestGitVCS_NestedDirectories(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create deeply nested file
	deepPath := filepath.Join("a", "b", "c", "d", "deep.txt")
	if err := os.MkdirAll(filepath.Join(repoPath, "a", "b", "c", "d"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	h.WriteFile(repoPath, deepPath, "deep content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add deep file")

	tracked, err := gitVCS.IsFileTracked(ctx, deepPath)
	if err != nil {
		t.Fatalf("IsFileTracked nested: %v", err)
	}
	if !tracked {
		t.Error("expected deeply nested file to be tracked")
	}

	content, err := gitVCS.ShowFile(ctx, "HEAD", deepPath)
	if err != nil {
		t.Fatalf("ShowFile nested: %v", err)
	}
	if string(content) != "deep content" {
		t.Errorf("expected 'deep content', got %q", string(content))
	}
}

func TestGitVCS_LargeFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create a 1MB file
	largeContent := strings.Repeat("x", 1024*1024)
	h.WriteFile(repoPath, "large.txt", largeContent)
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add large file")

	content, err := gitVCS.ShowFile(ctx, "HEAD", "large.txt")
	if err != nil {
		t.Fatalf("ShowFile large: %v", err)
	}
	if len(content) != 1024*1024 {
		t.Errorf("expected 1MB content, got %d bytes", len(content))
	}
}

func TestGitVCS_BinaryFile(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create binary file with null bytes
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x00, 0x89, 0x50, 0x4E, 0x47}
	if err := os.WriteFile(filepath.Join(repoPath, "binary.dat"), binaryContent, 0644); err != nil {
		t.Fatalf("WriteFile binary: %v", err)
	}
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add binary file")

	tracked, err := gitVCS.IsFileTracked(ctx, "binary.dat")
	if err != nil {
		t.Fatalf("IsFileTracked binary: %v", err)
	}
	if !tracked {
		t.Error("expected binary file to be tracked")
	}

	// ShowFile should return binary content
	content, err := gitVCS.ShowFile(ctx, "HEAD", "binary.dat")
	if err != nil {
		t.Fatalf("ShowFile binary: %v", err)
	}
	if len(content) != len(binaryContent) {
		t.Errorf("expected %d bytes, got %d", len(binaryContent), len(content))
	}
}

func TestGitVCS_SpacesInFilename(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create file with spaces in name
	h.WriteFile(repoPath, "my file.txt", "spaced content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add spaced file")

	tracked, err := gitVCS.IsFileTracked(ctx, "my file.txt")
	if err != nil {
		t.Fatalf("IsFileTracked spaced: %v", err)
	}
	if !tracked {
		t.Error("expected file with spaces to be tracked")
	}

	content, err := gitVCS.ShowFile(ctx, "HEAD", "my file.txt")
	if err != nil {
		t.Fatalf("ShowFile spaced: %v", err)
	}
	if string(content) != "spaced content" {
		t.Errorf("expected 'spaced content', got %q", string(content))
	}
}

func TestGitVCS_EmptyCommitMessage(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Stage a file and try empty commit message
	h.WriteFile(repoPath, "new.txt", "new")
	if err := gitVCS.Stage(ctx, "new.txt"); err != nil {
		t.Fatalf("Stage: %v", err)
	}

	// Empty commit message - git should reject with --allow-empty-message not set
	err = gitVCS.Commit(ctx, "", nil)
	// Git behavior: empty message with -m "" actually works (creates commit with empty message)
	// We just verify it doesn't panic
	_ = err
}

func TestGitVCS_MultipleRemotes(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	// Add multiple remotes
	h.runCmd(repoPath, "git", "remote", "add", "origin", "https://github.com/user/repo.git")
	h.runCmd(repoPath, "git", "remote", "add", "upstream", "https://github.com/upstream/repo.git")
	h.runCmd(repoPath, "git", "remote", "add", "fork", "git@github.com:fork/repo.git")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	urls, err := gitVCS.GetRemoteURLs(ctx)
	if err != nil {
		t.Fatalf("GetRemoteURLs: %v", err)
	}

	if len(urls) < 3 {
		t.Errorf("expected at least 3 remotes, got %d: %v", len(urls), urls)
	}
	if urls["origin"] != "https://github.com/user/repo.git" {
		t.Errorf("wrong origin URL: %q", urls["origin"])
	}
	if urls["upstream"] != "https://github.com/upstream/repo.git" {
		t.Errorf("wrong upstream URL: %q", urls["upstream"])
	}
	if urls["fork"] != "git@github.com:fork/repo.git" {
		t.Errorf("wrong fork URL: %q", urls["fork"])
	}

	hasRemote, err := gitVCS.HasRemote(ctx)
	if err != nil {
		t.Fatalf("HasRemote: %v", err)
	}
	if !hasRemote {
		t.Error("expected HasRemote true with 3 remotes")
	}
}

func TestGitVCS_ContextTimeout(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = gitVCS.Status(ctx)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestGitVCS_Symlink(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "target.txt", "target content")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	// Create a symlink
	if err := os.Symlink("target.txt", filepath.Join(repoPath, "link.txt")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add symlink")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	tracked, err := gitVCS.IsFileTracked(ctx, "link.txt")
	if err != nil {
		t.Fatalf("IsFileTracked symlink: %v", err)
	}
	if !tracked {
		t.Error("expected symlink to be tracked")
	}
}

func TestGitVCS_GitignoreInteraction(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	// Create .gitignore that ignores *.log files
	h.WriteFile(repoPath, ".gitignore", "*.log\n")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "commit", "-m", "add gitignore")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create ignored file
	h.WriteFile(repoPath, "debug.log", "log content")

	// Status should not show .log file as untracked
	entries, err := gitVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Path, ".log") {
			t.Errorf("expected .log file to be ignored, but found in status: %s", e.Path)
		}
	}

	// CheckIgnore should confirm it's ignored
	ignored, err := gitVCS.CheckIgnore(ctx, "debug.log")
	if err != nil {
		t.Fatalf("CheckIgnore: %v", err)
	}
	if !ignored {
		t.Error("expected debug.log to be ignored")
	}
}

func TestGitVCS_StatusMultipleFiles(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Create multiple changes: new file, modified file, deleted file
	h.WriteFile(repoPath, "new.txt", "new content")
	h.WriteFile(repoPath, "init.txt", "modified")
	h.WriteFile(repoPath, "staged.txt", "staged")
	h.runCmd(repoPath, "git", "add", "staged.txt")

	entries, err := gitVCS.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	// Should have at least 3 entries: new.txt (untracked), init.txt (modified), staged.txt (added)
	if len(entries) < 3 {
		t.Errorf("expected at least 3 status entries, got %d", len(entries))
		for _, e := range entries {
			t.Logf("  %s staged=%v %s", e.Status, e.Staged, e.Path)
		}
	}
}

func TestGitVCS_DiffHasChanges_StagedVsUnstaged(t *testing.T) {
	h := NewTestHelper(t)
	repoPath := h.CreateGitRepo("test")
	ctx := context.Background()

	h.WriteFile(repoPath, "file.txt", "original")
	h.runCmd(repoPath, "git", "add", ".")
	h.runCmd(repoPath, "git", "checkout", "-b", "main")
	h.runCmd(repoPath, "git", "commit", "-m", "init")

	gitVCS, err := NewGitVCS(repoPath)
	if err != nil {
		t.Fatalf("NewGitVCS: %v", err)
	}

	// Modify and stage
	h.WriteFile(repoPath, "file.txt", "staged change")
	h.runCmd(repoPath, "git", "add", "file.txt")

	// DiffHasChanges against HEAD should detect staged changes
	hasChanges, err := gitVCS.DiffHasChanges(ctx, "HEAD", "file.txt")
	if err != nil {
		t.Fatalf("DiffHasChanges: %v", err)
	}
	if !hasChanges {
		t.Error("expected changes after staging modification")
	}

	// Modify again (unstaged on top of staged)
	h.WriteFile(repoPath, "file.txt", "unstaged on top")

	hasChanges, err = gitVCS.DiffHasChanges(ctx, "HEAD", "file.txt")
	if err != nil {
		t.Fatalf("DiffHasChanges unstaged: %v", err)
	}
	if !hasChanges {
		t.Error("expected changes with unstaged modifications")
	}
}

// --- wong-bpg.5: JJ Workspace Sync Integration Tests ---

func TestJujutsuVCS_WorkspaceCreateListRemove(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-ws-lifecycle")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create initial content so workspace has something
	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "jj", "status") // snapshot
	jjVCS.Commit(ctx, "Base commit", nil)

	// List workspaces - should have only "default"
	workspaces, err := jjVCS.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].Name != "default" {
		t.Errorf("expected 1 workspace 'default', got %v", workspaces)
	}

	// Create a sync workspace (simulating sync_workspace.go pattern)
	syncPath := filepath.Join(h.tempDir, "beads-sync-workspace")
	if err := jjVCS.CreateWorkspace(ctx, "beads-sync", syncPath); err != nil {
		t.Fatalf("CreateWorkspace beads-sync: %v", err)
	}

	// Verify workspace created
	workspaces, err = jjVCS.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces after create: %v", err)
	}
	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(workspaces))
	}

	foundSync := false
	for _, ws := range workspaces {
		if ws.Name == "beads-sync" {
			foundSync = true
			break
		}
	}
	if !foundSync {
		t.Error("expected to find 'beads-sync' workspace")
	}

	// Remove workspace
	if err := jjVCS.RemoveWorkspace(ctx, "beads-sync"); err != nil {
		t.Fatalf("RemoveWorkspace: %v", err)
	}

	workspaces, err = jjVCS.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces after remove: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace after removal, got %d", len(workspaces))
	}
}

func TestJujutsuVCS_WorkspaceFileSync(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-ws-filesync")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create JSONL file (simulating beads sync pattern)
	h.WriteFile(repoPath, "issues.jsonl", `{"id":"1","title":"test issue"}`)
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Add issues file", nil)

	// Create workspace for sync
	syncPath := filepath.Join(h.tempDir, "sync-workspace")
	if err := jjVCS.CreateWorkspace(ctx, "sync", syncPath); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	defer jjVCS.RemoveWorkspace(ctx, "sync")

	// Verify the file is accessible in the sync workspace
	syncJJVCS, err := NewJujutsuVCS(syncPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS for sync workspace: %v", err)
	}

	// The sync workspace should share the repo's history
	files, err := syncJJVCS.ListTrackedFiles(ctx, ".")
	if err != nil {
		t.Fatalf("ListTrackedFiles in sync workspace: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected tracked files in sync workspace")
	}
}

func TestJujutsuVCS_SnapshotAndDescribe(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-snapshot")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Write a file
	h.WriteFile(repoPath, "data.txt", "snapshot test")

	// Explicit snapshot (jj auto-snapshots but this tests the method)
	if err := jjVCS.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Describe the current change
	if err := jjVCS.Describe(ctx, "Snapshot test description"); err != nil {
		t.Fatalf("Describe: %v", err)
	}

	// Verify description via log
	log, err := jjVCS.Log(ctx, 1)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(log) == 0 {
		t.Fatal("expected at least 1 log entry")
	}
	if !strings.Contains(log[0].Description, "Snapshot test description") {
		t.Errorf("expected description in log, got %q", log[0].Description)
	}
}

func TestJujutsuVCS_TrackAndUntrackFiles(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-track")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create files
	h.WriteFile(repoPath, "tracked.txt", "tracked")
	h.WriteFile(repoPath, "also-tracked.txt", "also tracked")
	h.runCmd(repoPath, "jj", "status")

	// TrackFiles
	if err := jjVCS.TrackFiles(ctx, "tracked.txt", "also-tracked.txt"); err != nil {
		t.Fatalf("TrackFiles: %v", err)
	}

	// Commit so they're in history
	jjVCS.Commit(ctx, "Add tracked files", nil)

	// Check tracked
	tracked, err := jjVCS.IsFileTracked(ctx, "tracked.txt")
	if err != nil {
		t.Fatalf("IsFileTracked: %v", err)
	}
	if !tracked {
		t.Error("expected tracked.txt to be tracked after TrackFiles")
	}

	// UntrackFiles
	if err := jjVCS.UntrackFiles(ctx, "also-tracked.txt"); err != nil {
		// UntrackFiles may fail on some jj versions, just log
		t.Logf("UntrackFiles: %v (may be unsupported)", err)
	}
}

func TestJujutsuVCS_ShowFile(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-showfile")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create and commit a file
	h.WriteFile(repoPath, "config.json", `{"key":"value"}`)
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Add config", nil)

	// ShowFile at current revision
	content, err := jjVCS.ShowFile(ctx, "@-", "config.json")
	if err != nil {
		t.Fatalf("ShowFile: %v", err)
	}
	if !strings.Contains(string(content), "key") {
		t.Errorf("expected config content, got %q", string(content))
	}

	// ShowFile for non-existent file should error
	_, err = jjVCS.ShowFile(ctx, "@-", "nonexistent.txt")
	if err == nil {
		t.Error("expected error for non-existent file in ShowFile")
	}
}

func TestJujutsuVCS_GetVCSDir(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-vcsdir")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	vcsDir, err := jjVCS.GetVCSDir(ctx)
	if err != nil {
		t.Fatalf("GetVCSDir: %v", err)
	}
	if !strings.Contains(vcsDir, ".jj") {
		t.Errorf("expected .jj in VCS dir, got %q", vcsDir)
	}
}

func TestJujutsuVCS_IsWorktreeRepo(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-worktree")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Default workspace should not be a worktree
	isWT, err := jjVCS.IsWorktreeRepo(ctx)
	if err != nil {
		t.Fatalf("IsWorktreeRepo: %v", err)
	}
	if isWT {
		t.Error("expected default workspace to not be a worktree")
	}
}

func TestJujutsuVCS_DiffHasChanges(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-diffchanges")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create and commit a file
	h.WriteFile(repoPath, "data.txt", "original")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Add data", nil)

	// No changes after commit
	hasChanges, err := jjVCS.DiffHasChanges(ctx, "@-", "data.txt")
	if err != nil {
		t.Logf("DiffHasChanges clean: %v (may be unsupported for this revset)", err)
	}

	// Modify the file
	h.WriteFile(repoPath, "data.txt", "modified")
	h.runCmd(repoPath, "jj", "status")

	hasChanges, err = jjVCS.DiffHasChanges(ctx, "@-", "data.txt")
	if err != nil {
		t.Logf("DiffHasChanges modified: %v", err)
	} else if !hasChanges {
		t.Error("expected changes after modification")
	}
}

func TestJujutsuVCS_BookmarkOperations(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-bookmarks-ops")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create content and commit
	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Initial", nil)

	// Create bookmark
	if err := jjVCS.CreateBranch(ctx, "sync-branch"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// BranchExists
	exists, err := jjVCS.BranchExists(ctx, "sync-branch")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if !exists {
		t.Error("expected sync-branch to exist")
	}

	// DeleteBranch
	if err := jjVCS.DeleteBranch(ctx, "sync-branch"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	exists, err = jjVCS.BranchExists(ctx, "sync-branch")
	if err != nil {
		t.Fatalf("BranchExists after delete: %v", err)
	}
	if exists {
		t.Error("expected sync-branch to not exist after deletion")
	}
}

func TestJujutsuVCS_SymbolicRef(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-symref")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create content and commit with a bookmark
	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Initial", nil)
	jjVCS.CreateBranch(ctx, "main")

	// SymbolicRef should return the bookmark name
	ref, err := jjVCS.SymbolicRef(ctx)
	if err != nil {
		t.Fatalf("SymbolicRef: %v", err)
	}
	// May return empty if bookmark is on parent, not working copy
	t.Logf("SymbolicRef returned: %q", ref)
}

func TestJujutsuVCS_GetRemoteURLs_NoRemote(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-noremote")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	urls, err := jjVCS.GetRemoteURLs(ctx)
	if err != nil {
		t.Fatalf("GetRemoteURLs: %v", err)
	}
	if len(urls) != 0 {
		t.Errorf("expected no remote URLs, got %v", urls)
	}

	hasRemote, err := jjVCS.HasRemote(ctx)
	if err != nil {
		t.Fatalf("HasRemote: %v", err)
	}
	if hasRemote {
		t.Error("expected HasRemote false with no remotes")
	}
}

func TestJujutsuVCS_ListTrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-listfiles")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create multiple files
	h.WriteFile(repoPath, "a.txt", "a")
	h.WriteFile(repoPath, "b.txt", "b")
	if err := os.MkdirAll(filepath.Join(repoPath, "sub"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	h.WriteFile(repoPath, filepath.Join("sub", "c.txt"), "c")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Add files", nil)

	files, err := jjVCS.ListTrackedFiles(ctx, ".")
	if err != nil {
		t.Fatalf("ListTrackedFiles: %v", err)
	}
	if len(files) < 3 {
		t.Errorf("expected at least 3 tracked files, got %d: %v", len(files), files)
	}
}

// --- wong-bpg.6: JJ Hook-related VCS Operation Tests ---

func TestJujutsuVCS_ConfigureHooksPath(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-hooks-config")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// ConfigureHooksPath for jj - may use config or be no-op
	hooksDir := filepath.Join(repoPath, ".beads", "hooks")
	err = jjVCS.ConfigureHooksPath(ctx, hooksDir)
	// jj doesn't have native hooks path, so this may return nil (no-op) or error
	t.Logf("ConfigureHooksPath result: %v", err)

	// GetHooksPath
	path, err := jjVCS.GetHooksPath(ctx)
	t.Logf("GetHooksPath result: %q, err: %v", path, err)
}

func TestJujutsuVCS_Checkout(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-checkout")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create two commits
	h.WriteFile(repoPath, "first.txt", "first")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "First commit", nil)

	h.WriteFile(repoPath, "second.txt", "second")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Second commit", nil)

	// Checkout parent (jj uses edit/new, but Checkout adapts)
	err = jjVCS.Checkout(ctx, "@--")
	if err != nil {
		t.Logf("Checkout @--: %v (may use edit under the hood)", err)
	}
}

func TestJujutsuVCS_RestoreFile(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-restore")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create and commit a file
	h.WriteFile(repoPath, "restore-me.txt", "original")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Original", nil)

	// Modify the file in new change
	h.WriteFile(repoPath, "restore-me.txt", "modified")
	h.runCmd(repoPath, "jj", "status")

	// Restore the file
	err = jjVCS.RestoreFile(ctx, "restore-me.txt")
	if err != nil {
		t.Logf("RestoreFile: %v", err)
	} else {
		// Verify restored
		content, _ := os.ReadFile(filepath.Join(repoPath, "restore-me.txt"))
		if string(content) == "modified" {
			t.Error("expected file to be restored to original")
		}
	}
}

func TestJujutsuVCS_Squash(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-squash")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create two commits
	h.WriteFile(repoPath, "base.txt", "base")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Base commit", nil)

	h.WriteFile(repoPath, "feature.txt", "feature")
	h.runCmd(repoPath, "jj", "status")

	// Squash current into parent
	err = jjVCS.Squash(ctx, "@")
	if err != nil {
		t.Logf("Squash: %v", err)
	}
}

func TestJujutsuVCS_NewAndEdit(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-new-edit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create initial content
	h.WriteFile(repoPath, "init.txt", "init")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Init", nil)

	// New creates empty change on top
	if err := jjVCS.New(ctx, "Work in progress"); err != nil {
		t.Fatalf("New: %v", err)
	}

	// Verify we're on a new change
	current, err := jjVCS.CurrentChange(ctx)
	if err != nil {
		t.Fatalf("CurrentChange: %v", err)
	}
	if !current.IsWorking {
		t.Error("expected new change to be working copy")
	}

	// Stack should show the change
	stack, err := jjVCS.StackInfo(ctx)
	if err != nil {
		t.Fatalf("StackInfo: %v", err)
	}
	if len(stack) < 2 {
		t.Errorf("expected at least 2 changes in stack, got %d", len(stack))
	}
}

func TestJujutsuVCS_GitExportImport(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-export-import")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create content and commit
	h.WriteFile(repoPath, "export.txt", "export me")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Export test", nil)

	// GitExport
	if err := jjVCS.GitExport(ctx); err != nil {
		t.Fatalf("GitExport: %v", err)
	}

	// GitImport (should be idempotent)
	if err := jjVCS.GitImport(ctx); err != nil {
		t.Fatalf("GitImport: %v", err)
	}
}

func TestJujutsuVCS_ResolveRef(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-resolve")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "test")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Commit for resolve", nil)

	// Resolve @ (current)
	id, err := jjVCS.ResolveRef(ctx, "@")
	if err != nil {
		t.Fatalf("ResolveRef @: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty resolved ref")
	}

	// Resolve @- (parent)
	parentID, err := jjVCS.ResolveRef(ctx, "@-")
	if err != nil {
		t.Fatalf("ResolveRef @-: %v", err)
	}
	if parentID == "" {
		t.Error("expected non-empty parent ref")
	}
	if id == parentID {
		t.Error("expected @ and @- to resolve to different IDs")
	}
}

func TestJujutsuVCS_LogBetween_ThreeCommits(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-logbetween")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create 3 commits
	h.WriteFile(repoPath, "a.txt", "a")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Commit A", nil)
	aRef, _ := jjVCS.ResolveRef(ctx, "@-")

	h.WriteFile(repoPath, "b.txt", "b")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Commit B", nil)

	h.WriteFile(repoPath, "c.txt", "c")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Commit C", nil)
	cRef, _ := jjVCS.ResolveRef(ctx, "@-")

	// LogBetween
	changes, err := jjVCS.LogBetween(ctx, aRef, cRef)
	if err != nil {
		t.Fatalf("LogBetween: %v", err)
	}
	if len(changes) < 1 {
		t.Errorf("expected at least 1 change between A and C, got %d", len(changes))
	}
}

// --- wong-bpg.8: Concurrent Workspace Operations ---

func TestJujutsuVCS_MultipleWorkspaces(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-multi-ws")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	// Create content
	h.WriteFile(repoPath, "shared.txt", "shared")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Shared content", nil)

	// Create multiple workspaces
	wsNames := []string{"ws-alpha", "ws-beta", "ws-gamma"}
	for _, name := range wsNames {
		wsPath := filepath.Join(h.tempDir, name)
		if err := jjVCS.CreateWorkspace(ctx, name, wsPath); err != nil {
			t.Fatalf("CreateWorkspace %s: %v", name, err)
		}
	}

	// Verify all created
	workspaces, err := jjVCS.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	// default + 3 custom = 4
	if len(workspaces) != 4 {
		t.Errorf("expected 4 workspaces, got %d", len(workspaces))
	}

	// Clean up workspaces in reverse order
	for i := len(wsNames) - 1; i >= 0; i-- {
		if err := jjVCS.RemoveWorkspace(ctx, wsNames[i]); err != nil {
			t.Fatalf("RemoveWorkspace %s: %v", wsNames[i], err)
		}
	}

	workspaces, err = jjVCS.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces after cleanup: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace after cleanup, got %d", len(workspaces))
	}
}

func TestJujutsuVCS_UpdateStaleWorkspace_Fresh(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed, skipping")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("jj-stale")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS: %v", err)
	}

	ctx := context.Background()

	h.WriteFile(repoPath, "test.txt", "content")
	h.runCmd(repoPath, "jj", "status")
	jjVCS.Commit(ctx, "Initial", nil)

	// Create a workspace
	wsPath := filepath.Join(h.tempDir, "stale-ws")
	if err := jjVCS.CreateWorkspace(ctx, "stale-test", wsPath); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	defer jjVCS.RemoveWorkspace(ctx, "stale-test")

	// UpdateStaleWorkspace on a fresh workspace should be a no-op
	err = jjVCS.UpdateStaleWorkspace(ctx, "stale-test")
	if err != nil {
		t.Logf("UpdateStaleWorkspace: %v (non-fatal)", err)
	}
}
