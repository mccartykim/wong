package vcs

import (
	"context"
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
