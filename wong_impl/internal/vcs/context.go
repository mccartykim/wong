// Package vcs provides context integration for VCS-agnostic beads operations.
// This file bridges the vcs package with the beads package's RepoContext.
package vcs

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
)

// RepoVCS holds the resolved VCS instance for a repository.
// This parallels beads.RepoContext but focuses on VCS operations.
type RepoVCS struct {
	// VCS is the version control system instance (Git or Jujutsu).
	VCS VCS

	// BeadsDir is the .beads directory path.
	BeadsDir string

	// RepoRoot is the repository root directory.
	RepoRoot string

	// IsColocated indicates this is a colocated jj+git repository.
	IsColocated bool
}

var (
	repoVCS     *RepoVCS
	repoVCSOnce sync.Once
	repoVCSErr  error
)

// GetRepoVCS returns the cached VCS context, initializing it on first call.
// It detects the VCS type and creates the appropriate backend.
func GetRepoVCS() (*RepoVCS, error) {
	repoVCSOnce.Do(func() {
		repoVCS, repoVCSErr = buildRepoVCS()
	})
	return repoVCS, repoVCSErr
}

// GetRepoVCSForPath returns a VCS context for a specific path.
// This doesn't use caching - use for testing or when path varies.
func GetRepoVCSForPath(path string) (*RepoVCS, error) {
	return buildRepoVCSForPath(path)
}

// buildRepoVCS constructs the RepoVCS by detecting VCS and creating backend.
func buildRepoVCS() (*RepoVCS, error) {
	// Start from current working directory
	cwd, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	return buildRepoVCSForPath(cwd)
}

// buildRepoVCSForPath constructs RepoVCS for a specific path.
func buildRepoVCSForPath(startPath string) (*RepoVCS, error) {
	// Detect VCS type
	vcsInstance, err := DetectVCS(startPath)
	if err != nil {
		return nil, err
	}

	repoRoot := vcsInstance.RepoRoot()

	// Look for .beads directory
	beadsDir := findBeadsDir(repoRoot)

	return &RepoVCS{
		VCS:         vcsInstance,
		BeadsDir:    beadsDir,
		RepoRoot:    repoRoot,
		IsColocated: vcsInstance.IsColocated(),
	}, nil
}

// findBeadsDir looks for a .beads directory starting from the given path.
func findBeadsDir(startPath string) string {
	current := startPath
	for {
		beadsPath := filepath.Join(current, ".beads")
		if isDirectory(beadsPath) {
			return beadsPath
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// ResetCaches clears the cached RepoVCS for testing.
func ResetVCSCaches() {
	repoVCSOnce = sync.Once{}
	repoVCS = nil
	repoVCSErr = nil
}

// --- Methods on RepoVCS for common operations ---

// Command creates an exec.Cmd for VCS operations in the repository.
// This is the primary method for VCS-agnostic command execution.
func (rv *RepoVCS) Command(ctx context.Context, args ...string) *exec.Cmd {
	return rv.VCS.Command(ctx, args...)
}

// Type returns the VCS type (git or jj).
func (rv *RepoVCS) Type() VCSType {
	return rv.VCS.Type()
}

// Status returns the working copy status.
func (rv *RepoVCS) Status(ctx context.Context) ([]StatusEntry, error) {
	return rv.VCS.Status(ctx)
}

// Stage stages files for commit.
func (rv *RepoVCS) Stage(ctx context.Context, paths ...string) error {
	return rv.VCS.Stage(ctx, paths...)
}

// Commit creates a commit with the given message.
func (rv *RepoVCS) Commit(ctx context.Context, message string, opts *CommitOptions) error {
	return rv.VCS.Commit(ctx, message, opts)
}

// Push pushes to remote.
func (rv *RepoVCS) Push(ctx context.Context, remote, branch string) error {
	return rv.VCS.Push(ctx, remote, branch)
}

// Pull fetches and merges from remote.
func (rv *RepoVCS) Pull(ctx context.Context, remote, branch string) error {
	return rv.VCS.Pull(ctx, remote, branch)
}

// Fetch fetches from remote without merging.
func (rv *RepoVCS) Fetch(ctx context.Context, remote, branch string) error {
	return rv.VCS.Fetch(ctx, remote, branch)
}

// CurrentBranch returns the current branch (git) or change ID (jj).
func (rv *RepoVCS) CurrentBranch(ctx context.Context) (string, error) {
	return rv.VCS.CurrentBranch(ctx)
}

// HasRemote checks if a remote is configured.
func (rv *RepoVCS) HasRemote(ctx context.Context) (bool, error) {
	return rv.VCS.HasRemote(ctx)
}

// GetRemote returns the default remote name.
func (rv *RepoVCS) GetRemote(ctx context.Context) (string, error) {
	return rv.VCS.GetRemote(ctx)
}

// HasMergeConflicts checks for unresolved conflicts.
func (rv *RepoVCS) HasMergeConflicts(ctx context.Context) (bool, error) {
	return rv.VCS.HasMergeConflicts(ctx)
}

// --- JJ-specific helpers for colocated repos ---

// IsJujutsu returns true if the VCS is Jujutsu.
func (rv *RepoVCS) IsJujutsu() bool {
	return rv.VCS.Type() == VCSTypeJujutsu
}

// IsGit returns true if the VCS is Git.
func (rv *RepoVCS) IsGit() bool {
	return rv.VCS.Type() == VCSTypeGit
}

// GitExport exports jj changes to git (colocated repos only).
// No-op for pure git repos.
func (rv *RepoVCS) GitExport(ctx context.Context) error {
	if jj, ok := rv.VCS.(*JujutsuVCS); ok {
		return jj.GitExport(ctx)
	}
	return nil
}

// GitImport imports git changes into jj (colocated repos only).
// No-op for pure git repos.
func (rv *RepoVCS) GitImport(ctx context.Context) error {
	if jj, ok := rv.VCS.(*JujutsuVCS); ok {
		return jj.GitImport(ctx)
	}
	return nil
}

// Snapshot forces a working copy snapshot (jj only).
// No-op for git.
func (rv *RepoVCS) Snapshot(ctx context.Context) error {
	if jj, ok := rv.VCS.(*JujutsuVCS); ok {
		return jj.Snapshot(ctx)
	}
	return nil
}

// StackInfo returns information about the current change stack.
// For git, returns unpushed commits. For jj, returns mutable changes.
func (rv *RepoVCS) StackInfo(ctx context.Context) ([]ChangeInfo, error) {
	return rv.VCS.StackInfo(ctx)
}

// --- Workspace/Worktree operations ---

// ListWorkspaces lists all workspaces (jj) or worktrees (git).
func (rv *RepoVCS) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	return rv.VCS.ListWorkspaces(ctx)
}

// CreateWorkspace creates a new workspace (jj) or worktree (git).
func (rv *RepoVCS) CreateWorkspace(ctx context.Context, name, path string) error {
	return rv.VCS.CreateWorkspace(ctx, name, path)
}

// RemoveWorkspace removes a workspace (jj) or worktree (git).
func (rv *RepoVCS) RemoveWorkspace(ctx context.Context, name string) error {
	return rv.VCS.RemoveWorkspace(ctx, name)
}

// --- Sync helpers for beads ---

// SyncCommit stages and commits beads files.
// This is the primary method for beads sync operations.
func (rv *RepoVCS) SyncCommit(ctx context.Context, message string, files ...string) error {
	// For jj, trigger a snapshot first to capture any working copy changes
	if rv.IsJujutsu() {
		if err := rv.Snapshot(ctx); err != nil {
			return err
		}
	}

	// Stage the files
	if err := rv.Stage(ctx, files...); err != nil {
		return err
	}

	// Commit
	return rv.Commit(ctx, message, &CommitOptions{
		NoGPGSign: true, // Don't require GPG signing for sync commits
	})
}

// SyncPush pushes beads changes to remote.
func (rv *RepoVCS) SyncPush(ctx context.Context) error {
	remote, err := rv.GetRemote(ctx)
	if err != nil {
		// No remote configured - that's ok for local-only repos
		return nil
	}

	branch, err := rv.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	return rv.Push(ctx, remote, branch)
}

// SyncPull pulls beads changes from remote.
func (rv *RepoVCS) SyncPull(ctx context.Context) error {
	remote, err := rv.GetRemote(ctx)
	if err != nil {
		// No remote configured
		return nil
	}

	branch, err := rv.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	return rv.Pull(ctx, remote, branch)
}

// SyncFull performs a complete sync: pull, stage, commit, push.
func (rv *RepoVCS) SyncFull(ctx context.Context, message string, files ...string) error {
	// 1. Pull latest changes
	if err := rv.SyncPull(ctx); err != nil {
		// Continue even if pull fails (might be no remote)
	}

	// 2. Commit local changes
	if err := rv.SyncCommit(ctx, message, files...); err != nil {
		return err
	}

	// 3. Push
	return rv.SyncPush(ctx)
}

// BeadsJSONLPath returns the path to the issues.jsonl file.
func (rv *RepoVCS) BeadsJSONLPath() string {
	if rv.BeadsDir == "" {
		return ""
	}
	return filepath.Join(rv.BeadsDir, "issues.jsonl")
}

// BeadsRelPath returns a path relative to the repo root.
func (rv *RepoVCS) BeadsRelPath(absPath string) (string, error) {
	return filepath.Rel(rv.RepoRoot, absPath)
}
