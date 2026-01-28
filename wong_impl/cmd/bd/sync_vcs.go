// sync_vcs.go provides VCS-agnostic sync operations for beads.
//
// This file parallels sync_git.go but works with both Git and Jujutsu.
// It uses the VCSContext from internal/beads to auto-detect the VCS backend.
//
// Migration path:
//   - Callers can switch from sync_git.go functions to these equivalents
//   - sync_git.go remains for backward compatibility
//   - New code should prefer these functions
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/vcs"
)

// isVCSRepo checks if the current working directory is in a VCS repository.
// Works for both git and jj.
func isVCSRepo() bool {
	_, err := vcs.FindRepoRoot(".")
	return err == nil
}

// vcsHasConflicts checks for unmerged/conflicted paths in the beads repository.
func vcsHasConflicts() (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}

	ctx := context.Background()
	return vc.VcsHasMergeConflicts(ctx)
}

// vcsHasUpstream checks if the repository has a remote configured.
func vcsHasUpstream() (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}

	ctx := context.Background()
	return vc.VcsHasRemote(ctx)
}

// vcsCurrentBranch returns the current branch (git) or change ID (jj).
func vcsCurrentBranch() (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}

	ctx := context.Background()
	return vc.VcsCurrentBranch(ctx)
}

// vcsGetRemote returns the default remote name.
func vcsGetRemote() (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}

	ctx := context.Background()
	return vc.VcsGetRemote(ctx)
}

// vcsFileStatus returns the status of a specific file.
func vcsFileStatus(path string) (*vcs.StatusEntry, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return nil, fmt.Errorf("getting VCS context: %w", err)
	}

	ctx := context.Background()
	return vc.VcsStatusPath(ctx, path)
}

// vcsBeadsDirStatus checks if .beads/ has uncommitted changes.
func vcsBeadsDirStatus() (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}

	ctx := context.Background()
	entries, err := vc.VcsStatus(ctx)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, ".beads/") || strings.HasPrefix(entry.Path, ".beads\\") {
			if entry.Status != vcs.FileStatusUnmodified {
				return true, nil
			}
		}
	}
	return false, nil
}

// vcsCommitFile stages and commits a single file.
func vcsCommitFile(ctx context.Context, path, message string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}

	// Make path relative to repo root
	relPath, err := filepath.Rel(vc.RepoRoot, path)
	if err != nil {
		relPath = path
	}

	// Stage
	if err := vc.VcsStage(ctx, relPath); err != nil {
		return fmt.Errorf("staging %s: %w", relPath, err)
	}

	// Commit
	opts := &vcs.CommitOptions{
		NoGPGSign: true,
		Paths:     []string{relPath},
	}
	return vc.VcsCommit(ctx, message, opts)
}

// vcsCommitBeadsDir stages and commits the .beads directory.
func vcsCommitBeadsDir(ctx context.Context, message string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}

	files := []string{
		".beads/issues.jsonl",
		".beads/deletions.jsonl",
		".beads/metadata.json",
	}

	// For jj, snapshot first
	if vc.IsJujutsu() {
		if jj, ok := vc.VCS.(*vcs.JujutsuVCS); ok {
			jj.Snapshot(ctx)
		}
	}

	// Stage all beads files
	for _, f := range files {
		if err := vc.VcsStage(ctx, f); err != nil {
			// Not all files may exist, continue
			continue
		}
	}

	// Commit
	opts := &vcs.CommitOptions{
		NoGPGSign: true,
		Paths:     files,
	}
	return vc.VcsCommit(ctx, message, opts)
}

// vcsPush pushes to the remote.
func vcsPush(ctx context.Context) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsPush(ctx)
}

// vcsPull pulls from the remote.
func vcsPull(ctx context.Context) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsPull(ctx)
}

// vcsFetch fetches from the remote without merging.
func vcsFetch(ctx context.Context) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsFetch(ctx)
}

// vcsGetFileVersion retrieves a specific version of a file (for conflict resolution).
func vcsGetFileVersion(ctx context.Context, path, version string) ([]byte, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return nil, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsGetFileVersion(ctx, path, version)
}

// vcsMarkResolved marks a file as resolved after conflict resolution.
func vcsMarkResolved(ctx context.Context, path string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsMarkResolved(ctx, path)
}

// --- Phase 1: Ref resolution, merge, config ---

// vcsBranchExists returns true if the named branch/bookmark exists.
func vcsBranchExists(ctx context.Context, name string) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsBranchExists(ctx, name)
}

// vcsResolveRef resolves a symbolic reference to a commit/change ID.
func vcsResolveRef(ctx context.Context, ref string) (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsResolveRef(ctx, ref)
}

// vcsIsAncestor returns true if ancestor is an ancestor of descendant.
func vcsIsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsIsAncestor(ctx, ancestor, descendant)
}

// vcsMerge merges the named branch/change into the current working copy.
func vcsMerge(ctx context.Context, branch, message string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsMerge(ctx, branch, message)
}

// vcsIsMerging returns true if a merge is in progress.
func vcsIsMerging(ctx context.Context) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsIsMerging(ctx)
}

// vcsGetConfig reads a VCS config value.
func vcsGetConfig(ctx context.Context, key string) (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsGetConfig(ctx, key)
}

// vcsSetConfig writes a VCS config value.
func vcsSetConfig(ctx context.Context, key, value string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsSetConfig(ctx, key, value)
}

// vcsGetRemoteURL returns the URL for a named remote.
func vcsGetRemoteURL(ctx context.Context, remote string) (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsGetRemoteURL(ctx, remote)
}

// vcsCheckoutFile checks out a specific file from a given revision.
func vcsCheckoutFile(ctx context.Context, ref, path string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsCheckoutFile(ctx, ref, path)
}

// vcsClean removes untracked files from the working copy.
func vcsClean(ctx context.Context) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsClean(ctx)
}

// vcsBranchHasUpstream checks if the current branch has upstream tracking.
func vcsBranchHasUpstream(ctx context.Context) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}

	branch, err := vc.VcsCurrentBranch(ctx)
	if err != nil {
		return false, err
	}

	if vc.IsJujutsu() {
		// jj bookmarks may track remotes - check if remote exists
		hasRemote, err := vc.VcsHasRemote(ctx)
		return hasRemote, err
	}

	// Git: check branch.{name}.remote config
	_, err = vc.VcsGetConfig(ctx, "branch."+branch+".remote")
	if err != nil {
		return false, nil // No upstream configured
	}
	return true, nil
}

// vcsHasBeadsChanges checks if .beads/ files have uncommitted changes.
func vcsHasBeadsChanges(ctx context.Context) (bool, error) {
	return vcsBeadsDirStatus()
}

// vcsHasFileChanges checks if a specific file has uncommitted changes.
func vcsHasFileChanges(ctx context.Context, path string) (bool, error) {
	entry, err := vcsFileStatus(path)
	if err != nil {
		return false, err
	}
	return entry.Status != vcs.FileStatusUnmodified, nil
}

// vcsGetDefaultBranch returns the default branch for the given remote.
func vcsGetDefaultBranch(ctx context.Context, remote string) (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}

	if vc.IsJujutsu() {
		// jj: check for main bookmark, then master
		if exists, _ := vc.VcsBranchExists(ctx, "main"); exists {
			return "main", nil
		}
		if exists, _ := vc.VcsBranchExists(ctx, "master"); exists {
			return "master", nil
		}
		return "main", nil
	}

	// Git: try symbolic-ref, then check main/master
	ref, err := vc.VcsResolveRef(ctx, "refs/remotes/"+remote+"/HEAD")
	if err == nil && ref != "" {
		parts := strings.Split(ref, "/")
		return parts[len(parts)-1], nil
	}

	// Fallback: check if main or master exists
	mainRef := remote + "/main"
	if _, err := vc.VcsResolveRef(ctx, mainRef); err == nil {
		return "main", nil
	}
	masterRef := remote + "/master"
	if _, err := vc.VcsResolveRef(ctx, masterRef); err == nil {
		return "master", nil
	}

	return "main", nil // default guess
}

// vcsFullSync performs a complete sync: pull, commit beads, push.
func vcsFullSync(ctx context.Context, message string) error {
	// 1. Pull latest
	if err := vcsPull(ctx); err != nil {
		// Continue even if pull fails (might be no remote)
	}

	// 2. Commit beads files
	if err := vcsCommitBeadsDir(ctx, message); err != nil {
		return err
	}

	// 3. Push
	return vcsPush(ctx)
}

// --- Phase 1c: Bridge functions for sync_git.go migration ---
// These functions match the signatures of sync_git.go functions so callers
// can switch from git* to vcs* with minimal code changes.

// vcsHasRemoteSimple returns true if a remote exists. Simple bool return for
// compatibility with hasGitRemote() callers.
func vcsHasRemoteSimple(ctx context.Context) bool {
	has, err := vcsHasUpstream()
	if err != nil {
		return false
	}
	return has
}

// vcsHasUpstreamSimple returns true if the current branch has upstream tracking.
// Simple bool return for compatibility with gitHasUpstream() callers.
func vcsHasUpstreamSimple() bool {
	ctx := context.Background()
	has, err := vcsBranchHasUpstream(ctx)
	if err != nil {
		return false
	}
	return has
}

// vcsNamedBranchHasUpstream checks if a specific named branch has upstream tracking.
// Matches gitBranchHasUpstream(branch) signature.
func vcsNamedBranchHasUpstream(branch string) bool {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false
	}

	ctx := context.Background()
	if vc.IsJujutsu() {
		// jj: check if remote exists (bookmarks track remotes)
		hasRemote, err := vc.VcsHasRemote(ctx)
		return err == nil && hasRemote
	}

	// Git: check branch.{name}.remote and branch.{name}.merge config
	_, remoteErr := vc.VcsGetConfig(ctx, "branch."+branch+".remote")
	_, mergeErr := vc.VcsGetConfig(ctx, "branch."+branch+".merge")
	return remoteErr == nil && mergeErr == nil
}

// vcsPullWithRemote pulls from the remote, optionally overriding the remote name.
// If configuredRemote is non-empty, uses that remote instead of the default.
// Matches gitPull(ctx, configuredRemote) signature.
func vcsPullWithRemote(ctx context.Context, configuredRemote string) error {
	// Check if any remote exists (support local-only repos)
	if !vcsHasRemoteSimple(ctx) {
		return nil // Gracefully skip - local-only mode
	}

	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}

	branch, err := vc.VcsCurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Determine remote: use configured override, or detect from VCS
	remote := configuredRemote
	if remote == "" {
		r, err := vc.VcsGetRemote(ctx)
		if err != nil {
			remote = "origin" // fallback
		} else {
			remote = r
		}
	}

	return vc.VCS.Pull(ctx, remote, branch)
}

// vcsPushWithRemote pushes to the remote, optionally overriding the remote name.
// If configuredRemote is non-empty, pushes to that remote explicitly.
// Matches gitPush(ctx, configuredRemote) signature.
func vcsPushWithRemote(ctx context.Context, configuredRemote string) error {
	// Check if any remote exists (support local-only repos)
	if !vcsHasRemoteSimple(ctx) {
		return nil // Gracefully skip - local-only mode
	}

	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}

	// If configuredRemote is set, push to that remote with current branch
	if configuredRemote != "" {
		branch, err := vc.VcsCurrentBranch(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
		return vc.VCS.Push(ctx, configuredRemote, branch)
	}

	// Default: use VCS default push behavior
	return vc.VcsPush(ctx)
}

// vcsHasUncommittedBeadsChanges checks if .beads/issues.jsonl has uncommitted changes.
// VCS-agnostic replacement for gitHasUncommittedBeadsChanges.
func vcsHasUncommittedBeadsChanges(ctx context.Context) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, nil // No VCS context, nothing to check
	}

	jsonlPath := ".beads/issues.jsonl"
	entry, err := vc.VcsStatusPath(ctx, jsonlPath)
	if err != nil {
		return false, fmt.Errorf("VCS status failed: %w", err)
	}

	return entry.Status != vcs.FileStatusUnmodified, nil
}

// vcsGetDefaultBranchForRemote returns the default branch for a specific remote.
// Returns a plain string (not error) for compatibility with getDefaultBranchForRemote.
func vcsGetDefaultBranchForRemote(ctx context.Context, remote string) string {
	branch, err := vcsGetDefaultBranch(ctx, remote)
	if err != nil {
		return "main"
	}
	return branch
}

// vcsFileStatusUnmodified returns the FileStatusUnmodified constant.
// Helper for callers that compare status without importing vcs package directly.
func vcsFileStatusUnmodified() vcs.FileStatus {
	return vcs.FileStatusUnmodified
}

// --- Phase 2: Sync-branch worktree/workspace operations ---
// These functions abstract the git worktree operations used in
// sync_branch.go and daemon_sync_branch.go for VCS-agnostic use.

// vcsGetContextForPath returns a VCSContext for an arbitrary directory.
// Used for operating on worktrees/workspaces separate from the main repo.
func vcsGetContextForPath(path string) (*beads.VCSContext, error) {
	return beads.GetVCSContextForPath(path)
}

// vcsLogBetween returns changes in 'to' not in 'from'.
// Replacement for: rc.GitCmd(ctx, "log", "--oneline", from+".."+to)
func vcsLogBetween(ctx context.Context, from, to string) ([]vcs.ChangeInfo, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return nil, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsLogBetween(ctx, from, to)
}

// vcsDiffPath returns the diff of a specific file between two refs.
// Replacement for: rc.GitCmd(ctx, "diff", from+"..."+to, "--", path)
func vcsDiffPath(ctx context.Context, from, to, path string) (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsDiffPath(ctx, from, to, path)
}

// vcsHasStagedChanges returns true if there are staged/pending changes.
// Replacement for: rc.GitCmd(ctx, "diff", "--cached", "--quiet")
func vcsHasStagedChanges(ctx context.Context) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsHasStagedChanges(ctx)
}

// vcsStageAndCommit stages files and commits atomically.
// Replacement for git add + git commit sequences in sync_branch.go.
func vcsStageAndCommit(ctx context.Context, paths []string, message string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	opts := &vcs.CommitOptions{NoGPGSign: true, Paths: paths}
	return vc.VcsStageAndCommit(ctx, paths, message, opts)
}

// vcsPushWithUpstream pushes with --set-upstream behavior.
// Replacement for: exec.Command("git", "-C", path, "push", "--set-upstream", remote, branch)
func vcsPushWithUpstream(ctx context.Context, remote, branch string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsPushWithUpstream(ctx, remote, branch)
}

// vcsRebase rebases current branch onto the given ref.
// Replacement for: exec.Command("git", "-C", path, "rebase", ref)
func vcsRebase(ctx context.Context, onto string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsRebase(ctx, onto)
}

// vcsRebaseAbort aborts a rebase in progress.
// Replacement for: exec.Command("git", "-C", path, "rebase", "--abort")
func vcsRebaseAbort(ctx context.Context) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsRebaseAbort(ctx)
}

// --- Path-aware worktree operations (operate on specific directory) ---

// vcsWorktreeStatus checks file status in a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "status", "--porcelain", relPath)
func vcsWorktreeStatus(ctx context.Context, worktreePath, filePath string) (*vcs.StatusEntry, error) {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.StatusPath(ctx, filePath)
}

// vcsWorktreeStageAndCommit stages and commits in a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "add" + "commit")
func vcsWorktreeStageAndCommit(ctx context.Context, worktreePath string, paths []string, message string) error {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	opts := &vcs.CommitOptions{NoGPGSign: true, Paths: paths}
	return vc.VCS.StageAndCommit(ctx, paths, message, opts)
}

// vcsWorktreePush pushes from a worktree/workspace with upstream tracking.
// Replacement for: exec.Command("git", "-C", worktreePath, "push", "--set-upstream", remote, branch)
func vcsWorktreePush(ctx context.Context, worktreePath, remote, branch string) error {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.PushWithUpstream(ctx, remote, branch)
}

// vcsWorktreePull pulls into a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "pull", remote, branch)
func vcsWorktreePull(ctx context.Context, worktreePath, remote, branch string) error {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.Pull(ctx, remote, branch)
}

// vcsWorktreeRebase rebases in a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "rebase", ref)
func vcsWorktreeRebase(ctx context.Context, worktreePath, onto string) error {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.Rebase(ctx, onto)
}

// vcsWorktreeRebaseAbort aborts a rebase in a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "rebase", "--abort")
func vcsWorktreeRebaseAbort(ctx context.Context, worktreePath string) error {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.RebaseAbort(ctx)
}

// vcsWorktreeGetConfig reads VCS config in a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "config", "--get", key)
func vcsWorktreeGetConfig(ctx context.Context, worktreePath, key string) (string, error) {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return "", fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.GetConfig(ctx, key)
}

// vcsWorktreeFetch fetches in a worktree/workspace.
// Replacement for: exec.Command("git", "-C", worktreePath, "fetch", remote, branch)
func vcsWorktreeFetch(ctx context.Context, worktreePath, remote, branch string) error {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return fmt.Errorf("VCS context for %s: %w", worktreePath, err)
	}
	return vc.VCS.Fetch(ctx, remote, branch)
}

// vcsWorktreeHasRemote checks if a worktree/workspace has a remote configured.
// Replacement for: exec.Command("git", "-C", worktreePath, "remote")
func vcsWorktreeHasRemote(ctx context.Context, worktreePath string) bool {
	vc, err := vcsGetContextForPath(worktreePath)
	if err != nil {
		return false
	}
	has, err := vc.VCS.HasRemote(ctx)
	if err != nil {
		return false
	}
	return has
}

// vcsGetRepoRoot returns the repo root for a given path.
// Replacement for: exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
func vcsGetRepoRoot(path string) (string, error) {
	vc, err := vcsGetContextForPath(path)
	if err != nil {
		return "", err
	}
	return vc.VCS.RepoRoot(), nil
}

// --- Phase 3: Hook integration bridge functions ---
// These functions abstract git hook operations (core.hooksPath, merge.beads.driver,
// ls-files --error-unmatch) for VCS-agnostic use.

// vcsIsFileTracked returns true if the file is tracked by VCS.
// Replacement for: exec.Command("git", "ls-files", "--error-unmatch", path)
func vcsIsFileTracked(ctx context.Context, path string) (bool, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return false, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsIsFileTracked(ctx, path)
}

// vcsConfigureHooksPath sets the hooks directory path.
// Replacement for: exec.Command("git", "config", "core.hooksPath", path)
func vcsConfigureHooksPath(ctx context.Context, path string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsConfigureHooksPath(ctx, path)
}

// vcsGetHooksPath returns the configured hooks path, or empty if default.
// Replacement for: exec.Command("git", "config", "--get", "core.hooksPath")
func vcsGetHooksPath(ctx context.Context) (string, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return "", fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsGetHooksPath(ctx)
}

// vcsConfigureMergeDriver sets up the custom merge driver for JSONL files.
// Replacement for: exec.Command("git", "config", "merge.beads.driver", cmd) +
//
//	exec.Command("git", "config", "merge.beads.name", name)
func vcsConfigureMergeDriver(ctx context.Context, driverCmd, driverName string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsConfigureMergeDriver(ctx, driverCmd, driverName)
}

// vcsStageFiles stages specific files for the next commit.
// Replacement for: rc.GitCmdCWD(ctx, "add", files...) in hook callbacks.
// For jj, this is a no-op (auto-snapshots).
func vcsStageFiles(ctx context.Context, paths ...string) error {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsStage(ctx, paths...)
}

// vcsStatusPorcelain returns the VCS status entries (replaces git status --porcelain).
func vcsStatusPorcelain(ctx context.Context) ([]vcs.StatusEntry, error) {
	vc, err := beads.GetVCSContext()
	if err != nil {
		return nil, fmt.Errorf("getting VCS context: %w", err)
	}
	return vc.VcsStatus(ctx)
}
