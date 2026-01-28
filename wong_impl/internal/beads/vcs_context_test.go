package beads

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/vcs"
)

// createTestGitRepo creates a git repo for testing.
func createTestGitRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "vcs-ctx-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}

	run("git", "init")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test User")
	run("git", "config", "commit.gpgsign", "false")

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	run("git", "add", ".")
	run("git", "checkout", "-b", "trunk")
	run("git", "commit", "-m", "initial commit")

	return dir
}

func TestGetVCSContextForPath_Git(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	if vc.VCSType != vcs.VCSTypeGit {
		t.Errorf("expected git VCS type, got %s", vc.VCSType)
	}
	if vc.IsJujutsu() {
		t.Error("expected IsJujutsu() to be false")
	}
	if !vc.IsGit() {
		t.Error("expected IsGit() to be true")
	}
	if vc.RepoContext != nil {
		t.Error("expected nil RepoContext for path-based context")
	}
}

func TestGetVCSContextForPath_InvalidPath(t *testing.T) {
	_, err := GetVCSContextForPath("/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestGetVCSContextForPath_NotARepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "not-a-repo-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(dir)

	_, err = GetVCSContextForPath(dir)
	if err == nil {
		t.Error("expected error for non-repo directory")
	}
}

func TestVCSContext_StatusWorkflow(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// Clean status
	entries, err := vc.VcsStatus(ctx)
	if err != nil {
		t.Fatalf("VcsStatus: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected clean status, got %d entries", len(entries))
	}

	// Create a new file
	if err := os.WriteFile(filepath.Join(repoPath, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err = vc.VcsStatus(ctx)
	if err != nil {
		t.Fatalf("VcsStatus after new file: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected non-empty status after creating file")
	}
}

func TestVCSContext_StageCommitWorkflow(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// Write, stage, commit
	if err := os.WriteFile(filepath.Join(repoPath, "staged.txt"), []byte("staged content"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := vc.VcsStage(ctx, "staged.txt"); err != nil {
		t.Fatalf("VcsStage: %v", err)
	}

	if err := vc.VcsCommit(ctx, "test commit via VCSContext", nil); err != nil {
		t.Fatalf("VcsCommit: %v", err)
	}

	// Verify clean
	entries, err := vc.VcsStatus(ctx)
	if err != nil {
		t.Fatalf("VcsStatus: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected clean after commit, got %d entries", len(entries))
	}
}

func TestVCSContext_CurrentBranch(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	branch, err := vc.VcsCurrentBranch(ctx)
	if err != nil {
		t.Fatalf("VcsCurrentBranch: %v", err)
	}
	if branch != "trunk" {
		t.Errorf("expected 'trunk', got %q", branch)
	}
}

func TestVCSContext_BranchOperations(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// Create branch
	if err := vc.VCS.CreateBranch(ctx, "feature-test"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	exists, err := vc.VCS.BranchExists(ctx, "feature-test")
	if err != nil || !exists {
		t.Fatal("expected feature-test to exist")
	}

	// Delete branch
	if err := vc.VcsDeleteBranch(ctx, "feature-test"); err != nil {
		t.Fatalf("VcsDeleteBranch: %v", err)
	}

	exists, _ = vc.VCS.BranchExists(ctx, "feature-test")
	if exists {
		t.Error("expected feature-test to be deleted")
	}
}

func TestVCSContext_ConfigOperations(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// Set and get config
	if err := vc.VCS.SetConfig(ctx, "test.ctx-key", "ctx-value"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	val, err := vc.VCS.GetConfig(ctx, "test.ctx-key")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "ctx-value" {
		t.Errorf("expected 'ctx-value', got %q", val)
	}
}

func TestVCSContext_FileOperations(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// IsFileTracked for committed file
	tracked, err := vc.VcsIsFileTracked(ctx, "init.txt")
	if err != nil {
		t.Fatalf("VcsIsFileTracked: %v", err)
	}
	if !tracked {
		t.Error("expected init.txt to be tracked")
	}

	// IsFileTracked for non-existent file
	tracked, err = vc.VcsIsFileTracked(ctx, "nonexistent.txt")
	if err != nil {
		t.Fatalf("VcsIsFileTracked nonexistent: %v", err)
	}
	if tracked {
		t.Error("expected nonexistent.txt to not be tracked")
	}

	// ListTrackedFiles
	files, err := vc.VcsListTrackedFiles(ctx, ".")
	if err != nil {
		t.Fatalf("VcsListTrackedFiles: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected at least 1 tracked file")
	}
}

func TestVCSContext_ShowFile(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	content, err := vc.VcsShowFile(ctx, "HEAD", "init.txt")
	if err != nil {
		t.Fatalf("VcsShowFile: %v", err)
	}
	if string(content) != "init" {
		t.Errorf("expected 'init', got %q", string(content))
	}
}

func TestVCSContext_VCSDir(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	vcsDir, err := vc.VcsGetVCSDir(ctx)
	if err != nil {
		t.Fatalf("VcsGetVCSDir: %v", err)
	}
	if !strings.HasSuffix(vcsDir, ".git") {
		t.Errorf("expected VCS dir to end with .git, got %q", vcsDir)
	}

	isWorktree, err := vc.VcsIsWorktreeRepo(ctx)
	if err != nil {
		t.Fatalf("VcsIsWorktreeRepo: %v", err)
	}
	if isWorktree {
		t.Error("expected non-worktree repo")
	}
}

func TestVCSContext_CheckoutAndSymbolicRef(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// Create a second branch
	if err := vc.VCS.CreateBranch(ctx, "alt-branch"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	// Checkout alt-branch
	if err := vc.VcsCheckout(ctx, "alt-branch"); err != nil {
		t.Fatalf("VcsCheckout: %v", err)
	}

	// SymbolicRef should return alt-branch
	ref, err := vc.VcsSymbolicRef(ctx)
	if err != nil {
		t.Fatalf("VcsSymbolicRef: %v", err)
	}
	if ref != "alt-branch" {
		t.Errorf("expected 'alt-branch', got %q", ref)
	}
}

func TestVCSContext_RemoteURLs(t *testing.T) {
	repoPath := createTestGitRepo(t)

	// Add a remote
	cmd := exec.Command("git", "remote", "add", "origin", "https://example.com/test.git")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("git remote add: %v", err)
	}

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	urls, err := vc.VcsGetRemoteURLs(ctx)
	if err != nil {
		t.Fatalf("VcsGetRemoteURLs: %v", err)
	}
	if url, ok := urls["origin"]; !ok || url != "https://example.com/test.git" {
		t.Errorf("expected origin URL, got %v", urls)
	}

	hasRemote, err := vc.VcsHasRemote(ctx)
	if err != nil {
		t.Fatalf("VcsHasRemote: %v", err)
	}
	if !hasRemote {
		t.Error("expected HasRemote to be true")
	}
}

func TestVCSContext_VcsCmd(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// Low-level command escape hatch
	cmd := vc.VcsCmd(ctx, "log", "--oneline", "-1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("VcsCmd: %v", err)
	}
	if !strings.Contains(string(out), "initial commit") {
		t.Errorf("expected 'initial commit' in output, got %q", string(out))
	}
}

func TestVCSContext_DiffHasChanges(t *testing.T) {
	repoPath := createTestGitRepo(t)

	vc, err := GetVCSContextForPath(repoPath)
	if err != nil {
		t.Fatalf("GetVCSContextForPath: %v", err)
	}

	ctx := context.Background()

	// No changes
	hasChanges, err := vc.VcsDiffHasChanges(ctx, "HEAD", "init.txt")
	if err != nil {
		t.Fatalf("VcsDiffHasChanges: %v", err)
	}
	if hasChanges {
		t.Error("expected no changes for clean file")
	}

	// Modify file
	if err := os.WriteFile(filepath.Join(repoPath, "init.txt"), []byte("modified"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	hasChanges, err = vc.VcsDiffHasChanges(ctx, "HEAD", "init.txt")
	if err != nil {
		t.Fatalf("VcsDiffHasChanges after modify: %v", err)
	}
	if !hasChanges {
		t.Error("expected changes after modification")
	}
}
