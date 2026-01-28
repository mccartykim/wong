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
