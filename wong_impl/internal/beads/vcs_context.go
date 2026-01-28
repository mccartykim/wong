// Package beads provides VCS-agnostic extensions to RepoContext.
//
// This file extends the existing RepoContext to support both Git and Jujutsu (jj)
// via the internal/vcs package. It provides backward-compatible methods that
// automatically detect and use the appropriate VCS backend.
//
// Existing code using rc.GitCmd() continues to work unchanged.
// New code should prefer rc.VcsCmd() for VCS-agnostic operations.
package beads

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/steveyegge/beads/internal/vcs"
)

// VCSContext extends RepoContext with VCS-agnostic operations.
// This wraps RepoContext to add jj support while keeping full backward compatibility.
type VCSContext struct {
	*RepoContext

	// VCS is the detected version control system instance.
	VCS vcs.VCS

	// VCSType is the type of VCS (git or jj).
	VCSType vcs.VCSType

	// IsColocated is true if this is a colocated jj+git repository.
	IsColocatedRepo bool
}

// GetVCSContext returns a VCSContext, auto-detecting the VCS type.
// This is the preferred entry point for new code that needs VCS-agnostic operations.
// Falls back to Git if VCS detection fails (backward compatible).
func GetVCSContext() (*VCSContext, error) {
	// Get the standard RepoContext first (for backward compat)
	rc, err := GetRepoContext()
	if err != nil {
		return nil, err
	}

	// Try to detect VCS
	vcsInstance, vcsErr := vcs.DetectVCS(rc.RepoRoot)
	if vcsErr != nil {
		// Fall back to Git (original behavior)
		gitVCS, gitErr := vcs.NewGitVCS(rc.RepoRoot)
		if gitErr != nil {
			return nil, fmt.Errorf("no VCS detected: %w (git fallback: %w)", vcsErr, gitErr)
		}
		return &VCSContext{
			RepoContext:     rc,
			VCS:             gitVCS,
			VCSType:         vcs.VCSTypeGit,
			IsColocatedRepo: false,
		}, nil
	}

	colocated, _ := vcs.IsColocatedRepo(rc.RepoRoot)

	return &VCSContext{
		RepoContext:     rc,
		VCS:             vcsInstance,
		VCSType:         vcsInstance.Type(),
		IsColocatedRepo: colocated,
	}, nil
}

// GetVCSContextForPath returns a VCSContext for an arbitrary directory path.
// This is used for operating on worktrees/workspaces that are separate from
// the main repo (e.g., sync-branch worktrees, parallel task workspaces).
func GetVCSContextForPath(path string) (*VCSContext, error) {
	vcsInstance, err := vcs.DetectVCS(path)
	if err != nil {
		return nil, fmt.Errorf("no VCS detected at %s: %w", path, err)
	}

	colocated, _ := vcs.IsColocatedRepo(path)

	return &VCSContext{
		RepoContext:     nil, // No RepoContext for external paths
		VCS:             vcsInstance,
		VCSType:         vcsInstance.Type(),
		IsColocatedRepo: colocated,
	}, nil
}

// VcsCmd creates an exec.Cmd for the detected VCS (git or jj).
// This is the VCS-agnostic replacement for GitCmd.
//
// For git repos: equivalent to GitCmd
// For jj repos: runs jj commands instead
func (vc *VCSContext) VcsCmd(ctx context.Context, args ...string) *exec.Cmd {
	return vc.VCS.Command(ctx, args...)
}

// IsJujutsu returns true if the detected VCS is Jujutsu.
func (vc *VCSContext) IsJujutsu() bool {
	return vc.VCSType == vcs.VCSTypeJujutsu
}

// IsGit returns true if the detected VCS is Git.
func (vc *VCSContext) IsGit() bool {
	return vc.VCSType == vcs.VCSTypeGit
}

// --- VCS-agnostic sync operations ---

// VcsStatus returns the working copy status using the detected VCS.
func (vc *VCSContext) VcsStatus(ctx context.Context) ([]vcs.StatusEntry, error) {
	return vc.VCS.Status(ctx)
}

// VcsStatusPath returns the status of a specific path.
func (vc *VCSContext) VcsStatusPath(ctx context.Context, path string) (*vcs.StatusEntry, error) {
	return vc.VCS.StatusPath(ctx, path)
}

// VcsCurrentBranch returns the current branch (git) or change ID (jj).
func (vc *VCSContext) VcsCurrentBranch(ctx context.Context) (string, error) {
	return vc.VCS.CurrentBranch(ctx)
}

// VcsHasRemote returns true if a remote is configured.
func (vc *VCSContext) VcsHasRemote(ctx context.Context) (bool, error) {
	return vc.VCS.HasRemote(ctx)
}

// VcsGetRemote returns the default remote name.
func (vc *VCSContext) VcsGetRemote(ctx context.Context) (string, error) {
	return vc.VCS.GetRemote(ctx)
}

// VcsStage stages files. No-op for jj (auto-snapshots).
func (vc *VCSContext) VcsStage(ctx context.Context, paths ...string) error {
	return vc.VCS.Stage(ctx, paths...)
}

// VcsCommit creates a new commit/change.
func (vc *VCSContext) VcsCommit(ctx context.Context, message string, opts *vcs.CommitOptions) error {
	return vc.VCS.Commit(ctx, message, opts)
}

// VcsPush pushes to the remote.
func (vc *VCSContext) VcsPush(ctx context.Context) error {
	remote, err := vc.VcsGetRemote(ctx)
	if err != nil {
		return nil // No remote, that's ok
	}
	branch, err := vc.VcsCurrentBranch(ctx)
	if err != nil {
		return err
	}
	return vc.VCS.Push(ctx, remote, branch)
}

// VcsPull fetches and integrates from the remote.
func (vc *VCSContext) VcsPull(ctx context.Context) error {
	remote, err := vc.VcsGetRemote(ctx)
	if err != nil {
		return nil
	}
	branch, err := vc.VcsCurrentBranch(ctx)
	if err != nil {
		return err
	}
	return vc.VCS.Pull(ctx, remote, branch)
}

// VcsFetch fetches from the remote without merging.
func (vc *VCSContext) VcsFetch(ctx context.Context) error {
	remote, err := vc.VcsGetRemote(ctx)
	if err != nil {
		return nil
	}
	branch, err := vc.VcsCurrentBranch(ctx)
	if err != nil {
		return err
	}
	return vc.VCS.Fetch(ctx, remote, branch)
}

// VcsHasMergeConflicts checks for unresolved conflicts.
func (vc *VCSContext) VcsHasMergeConflicts(ctx context.Context) (bool, error) {
	return vc.VCS.HasMergeConflicts(ctx)
}

// VcsGetFileVersion retrieves a specific version of a file.
func (vc *VCSContext) VcsGetFileVersion(ctx context.Context, path, version string) ([]byte, error) {
	return vc.VCS.GetFileVersion(ctx, path, version)
}

// VcsMarkResolved marks a file as resolved.
func (vc *VCSContext) VcsMarkResolved(ctx context.Context, path string) error {
	return vc.VCS.MarkResolved(ctx, path)
}

// --- Phase 1: Ref resolution, merge, config ---

// VcsBranchExists returns true if the named branch/bookmark exists.
func (vc *VCSContext) VcsBranchExists(ctx context.Context, name string) (bool, error) {
	return vc.VCS.BranchExists(ctx, name)
}

// VcsResolveRef resolves a symbolic reference to a commit/change ID.
func (vc *VCSContext) VcsResolveRef(ctx context.Context, ref string) (string, error) {
	return vc.VCS.ResolveRef(ctx, ref)
}

// VcsIsAncestor returns true if ancestor is an ancestor of descendant.
func (vc *VCSContext) VcsIsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	return vc.VCS.IsAncestor(ctx, ancestor, descendant)
}

// VcsMerge merges the named branch/change into the current working copy.
func (vc *VCSContext) VcsMerge(ctx context.Context, branch, message string) error {
	return vc.VCS.Merge(ctx, branch, message)
}

// VcsIsMerging returns true if a merge is in progress.
func (vc *VCSContext) VcsIsMerging(ctx context.Context) (bool, error) {
	return vc.VCS.IsMerging(ctx)
}

// VcsGetConfig reads a VCS config value.
func (vc *VCSContext) VcsGetConfig(ctx context.Context, key string) (string, error) {
	return vc.VCS.GetConfig(ctx, key)
}

// VcsSetConfig writes a VCS config value.
func (vc *VCSContext) VcsSetConfig(ctx context.Context, key, value string) error {
	return vc.VCS.SetConfig(ctx, key, value)
}

// VcsGetRemoteURL returns the URL for a named remote.
func (vc *VCSContext) VcsGetRemoteURL(ctx context.Context, remote string) (string, error) {
	return vc.VCS.GetRemoteURL(ctx, remote)
}

// VcsCheckoutFile checks out a specific file from a given revision.
func (vc *VCSContext) VcsCheckoutFile(ctx context.Context, ref, path string) error {
	return vc.VCS.CheckoutFile(ctx, ref, path)
}

// VcsClean removes untracked files from the working copy.
func (vc *VCSContext) VcsClean(ctx context.Context) error {
	return vc.VCS.Clean(ctx)
}

// --- Sync helpers specific to beads ---

// SyncBeadsFiles stages, commits, and optionally pushes .beads/ files.
// This is the primary sync method for beads operations.
func (vc *VCSContext) SyncBeadsFiles(ctx context.Context, message string, push bool) error {
	// For jj, snapshot working copy first
	if jj, ok := vc.VCS.(*vcs.JujutsuVCS); ok {
		jj.Snapshot(ctx)
	}

	// Stage beads files
	files := []string{
		".beads/issues.jsonl",
		".beads/deletions.jsonl",
		".beads/metadata.json",
	}
	if err := vc.VcsStage(ctx, files...); err != nil {
		return fmt.Errorf("failed to stage beads files: %w", err)
	}

	// Commit
	opts := &vcs.CommitOptions{
		NoGPGSign: true,
		Paths:     files,
	}
	if err := vc.VcsCommit(ctx, message, opts); err != nil {
		return fmt.Errorf("failed to commit beads files: %w", err)
	}

	// Push if requested
	if push {
		if err := vc.VcsPush(ctx); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
	}

	// For colocated repos, export to git after committing in jj
	if vc.IsColocatedRepo && vc.IsJujutsu() {
		if jj, ok := vc.VCS.(*vcs.JujutsuVCS); ok {
			jj.GitExport(ctx)
		}
	}

	return nil
}

// --- Migration helpers ---

// MigrateFromGitCmd provides a helper for migrating code from GitCmd to VcsCmd.
// It logs a deprecation notice when git-specific operations are used on jj repos.
func (vc *VCSContext) MigrateFromGitCmd(ctx context.Context, args ...string) *exec.Cmd {
	if vc.IsJujutsu() {
		// For jj repos, try to map common git commands
		return vc.mapGitToJJ(ctx, args...)
	}
	// For git repos, use GitCmd directly
	return vc.GitCmd(ctx, args...)
}

// --- Phase 2: Sync-branch worktree/workspace wrappers ---

// VcsLogBetween returns commits/changes in 'to' not in 'from'.
func (vc *VCSContext) VcsLogBetween(ctx context.Context, from, to string) ([]vcs.ChangeInfo, error) {
	return vc.VCS.LogBetween(ctx, from, to)
}

// VcsDiffPath returns the diff of a specific file between two refs.
func (vc *VCSContext) VcsDiffPath(ctx context.Context, from, to, path string) (string, error) {
	return vc.VCS.DiffPath(ctx, from, to, path)
}

// VcsHasStagedChanges returns true if there are staged/pending changes.
func (vc *VCSContext) VcsHasStagedChanges(ctx context.Context) (bool, error) {
	return vc.VCS.HasStagedChanges(ctx)
}

// VcsStageAndCommit stages files and commits atomically.
func (vc *VCSContext) VcsStageAndCommit(ctx context.Context, paths []string, message string, opts *vcs.CommitOptions) error {
	return vc.VCS.StageAndCommit(ctx, paths, message, opts)
}

// VcsPushWithUpstream pushes with --set-upstream behavior.
func (vc *VCSContext) VcsPushWithUpstream(ctx context.Context, remote, branch string) error {
	return vc.VCS.PushWithUpstream(ctx, remote, branch)
}

// VcsRebase rebases the current branch onto the given ref.
func (vc *VCSContext) VcsRebase(ctx context.Context, onto string) error {
	return vc.VCS.Rebase(ctx, onto)
}

// VcsRebaseAbort aborts a rebase in progress.
func (vc *VCSContext) VcsRebaseAbort(ctx context.Context) error {
	return vc.VCS.RebaseAbort(ctx)
}

// mapGitToJJ translates common git commands to jj equivalents.
func (vc *VCSContext) mapGitToJJ(ctx context.Context, args ...string) *exec.Cmd {
	if len(args) == 0 {
		return vc.VCS.Command(ctx, "status")
	}

	switch args[0] {
	case "status":
		return vc.VCS.Command(ctx, "status")
	case "add":
		// jj doesn't need add - auto-snapshots
		return vc.VCS.Command(ctx, "status") // no-op equivalent
	case "commit":
		jjArgs := []string{"commit"}
		for i := 1; i < len(args); i++ {
			if args[i] == "-m" && i+1 < len(args) {
				jjArgs = append(jjArgs, "-m", args[i+1])
				i++
			}
		}
		return vc.VCS.Command(ctx, jjArgs...)
	case "push":
		return vc.VCS.Command(ctx, "git", "push")
	case "pull":
		return vc.VCS.Command(ctx, "git", "fetch")
	case "fetch":
		jjArgs := []string{"git", "fetch"}
		return vc.VCS.Command(ctx, jjArgs...)
	case "log":
		return vc.VCS.Command(ctx, "log")
	case "diff":
		return vc.VCS.Command(ctx, "diff")
	case "symbolic-ref":
		// Map to jj's working copy info
		return vc.VCS.Command(ctx, "log", "-r", "@", "--no-graph", "-T", "change_id.short()")
	case "rev-parse":
		if len(args) > 1 && args[1] == "--git-dir" {
			// Return .jj path
			return vc.VCS.Command(ctx, "workspace", "root")
		}
		return vc.VCS.Command(ctx, "log", "-r", "@", "--no-graph", "-T", "commit_id.short()")
	case "remote":
		return vc.VCS.Command(ctx, "git", "remote", "list")
	default:
		// Unknown command - pass through to git (works for colocated or pure git)
		return vc.GitCmd(ctx, args...)
	}
}
