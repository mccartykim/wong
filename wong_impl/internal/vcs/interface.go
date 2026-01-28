// Package vcs provides an abstraction layer for version control systems.
// It supports both Git and Jujutsu (jj) backends, allowing beads to work
// seamlessly with either VCS.
package vcs

import (
	"context"
	"os/exec"
)

// VCSType identifies the version control system in use.
type VCSType string

const (
	VCSTypeGit     VCSType = "git"
	VCSTypeJujutsu VCSType = "jj"
	VCSTypeUnknown VCSType = "unknown"
)

// FileStatus represents the status of a file in the working copy.
type FileStatus string

const (
	FileStatusUnmodified FileStatus = "unmodified"
	FileStatusModified   FileStatus = "modified"
	FileStatusAdded      FileStatus = "added"
	FileStatusDeleted    FileStatus = "deleted"
	FileStatusRenamed    FileStatus = "renamed"
	FileStatusCopied     FileStatus = "copied"
	FileStatusUntracked  FileStatus = "untracked"
	FileStatusIgnored    FileStatus = "ignored"
	FileStatusConflicted FileStatus = "conflicted"
)

// StatusEntry represents the status of a single file.
type StatusEntry struct {
	Path      string
	Status    FileStatus
	OldPath   string // For renames/copies
	Staged    bool   // Whether change is staged (git) or snapshotted (jj)
	Conflicted bool
}

// BranchInfo represents information about a branch or bookmark.
type BranchInfo struct {
	Name       string
	IsCurrent  bool
	RemoteName string // Empty if local only
	Upstream   string // Upstream tracking ref
}

// ChangeInfo represents a commit (git) or change (jj).
type ChangeInfo struct {
	ID          string // Commit hash (git) or change ID (jj)
	ShortID     string // Short form of ID
	Description string
	Author      string
	Timestamp   string
	IsWorking   bool // True if this is the working copy (@ in jj)
}

// WorkspaceInfo represents a workspace (jj) or worktree (git).
type WorkspaceInfo struct {
	Name     string
	Path     string
	ChangeID string // Current change/commit in workspace
}

// MergeConflict represents a file with merge conflicts.
type MergeConflict struct {
	Path      string
	BaseBlob  string // Ancestor version
	OursBlob  string // Our version
	TheirsBlob string // Their version
}

// VCS is the primary interface for version control operations.
// Implementations must be safe for concurrent use.
type VCS interface {
	// Type returns the VCS type (git or jj).
	Type() VCSType

	// RepoRoot returns the root directory of the repository.
	RepoRoot() string

	// IsColocated returns true if this is a colocated jj+git repo.
	IsColocated() bool

	// Command creates an exec.Cmd for running VCS commands.
	// This is the low-level escape hatch for operations not covered by the interface.
	Command(ctx context.Context, args ...string) *exec.Cmd

	// --- Repository State ---

	// CurrentBranch returns the current branch (git) or current change description (jj).
	// For jj, this returns the working copy's change ID.
	CurrentBranch(ctx context.Context) (string, error)

	// CurrentChange returns info about the current change/commit.
	CurrentChange(ctx context.Context) (*ChangeInfo, error)

	// Status returns the working copy status.
	Status(ctx context.Context) ([]StatusEntry, error)

	// StatusPath returns the status of a specific path.
	StatusPath(ctx context.Context, path string) (*StatusEntry, error)

	// HasRemote returns true if a remote is configured.
	HasRemote(ctx context.Context) (bool, error)

	// GetRemote returns the default remote name (usually "origin").
	GetRemote(ctx context.Context) (string, error)

	// --- Staging & Committing ---

	// Stage stages files for the next commit.
	// For jj, this is a no-op as jj auto-snapshots the working copy.
	Stage(ctx context.Context, paths ...string) error

	// Commit creates a new commit with the given message.
	// For jj, this creates a new change and starts a fresh working copy.
	Commit(ctx context.Context, message string, opts *CommitOptions) error

	// --- Sync Operations ---

	// Fetch fetches from the remote without merging.
	Fetch(ctx context.Context, remote, branch string) error

	// Pull fetches and merges/rebases from the remote.
	Pull(ctx context.Context, remote, branch string) error

	// Push pushes to the remote.
	Push(ctx context.Context, remote, branch string) error

	// --- Branch/Bookmark Operations ---

	// ListBranches lists all branches (git) or bookmarks (jj).
	ListBranches(ctx context.Context) ([]BranchInfo, error)

	// CreateBranch creates a new branch (git) or bookmark (jj).
	CreateBranch(ctx context.Context, name string) error

	// SwitchBranch switches to a different branch (git checkout) or change (jj edit).
	SwitchBranch(ctx context.Context, name string) error

	// --- Workspace Operations ---

	// These mirror git worktree operations but also support jj workspaces.

	// ListWorkspaces lists all workspaces/worktrees.
	ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error)

	// CreateWorkspace creates a new workspace/worktree.
	CreateWorkspace(ctx context.Context, name, path string) error

	// RemoveWorkspace removes a workspace/worktree.
	RemoveWorkspace(ctx context.Context, name string) error

	// --- Merge & Conflict Operations ---

	// HasMergeConflicts returns true if there are unresolved merge conflicts.
	HasMergeConflicts(ctx context.Context) (bool, error)

	// GetConflicts returns information about merge conflicts.
	GetConflicts(ctx context.Context) ([]MergeConflict, error)

	// GetFileVersion retrieves a specific version of a file.
	// For merge conflicts: stage 1=base, 2=ours, 3=theirs.
	GetFileVersion(ctx context.Context, path string, version string) ([]byte, error)

	// MarkResolved marks a file as resolved.
	MarkResolved(ctx context.Context, path string) error

	// --- History Operations ---

	// Log returns recent changes/commits.
	Log(ctx context.Context, limit int) ([]ChangeInfo, error)

	// Show returns details of a specific change/commit.
	Show(ctx context.Context, id string) (*ChangeInfo, error)

	// Diff returns the diff between two revisions.
	Diff(ctx context.Context, from, to string) (string, error)

	// --- JJ-Specific Stacked Changes ---

	// These methods support jj's unique stacked changes model.
	// For git, they return appropriate equivalents or errors.

	// StackInfo returns information about the current change stack.
	// For git, this returns the current branch's unpushed commits.
	StackInfo(ctx context.Context) ([]ChangeInfo, error)

	// Squash squashes changes. For git: git commit --amend. For jj: jj squash.
	Squash(ctx context.Context, sourceID string) error

	// New creates a new change on top of the current one.
	// For git: no-op (commits are created with Commit).
	// For jj: jj new.
	New(ctx context.Context, message string) error

	// Edit sets a change as the working copy target.
	// For git: git checkout. For jj: jj edit.
	Edit(ctx context.Context, id string) error

	// --- Ref Resolution & Branch Queries ---

	// BranchExists returns true if the named branch/bookmark exists.
	BranchExists(ctx context.Context, name string) (bool, error)

	// ResolveRef resolves a symbolic reference to a commit/change ID.
	ResolveRef(ctx context.Context, ref string) (string, error)

	// IsAncestor returns true if ancestor is an ancestor of descendant.
	IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)

	// --- Merge Operations ---

	// Merge merges the named branch/change into the current working copy.
	Merge(ctx context.Context, branch, message string) error

	// IsMerging returns true if a merge is in progress.
	IsMerging(ctx context.Context) (bool, error)

	// --- Configuration ---

	// GetConfig reads a VCS config value.
	GetConfig(ctx context.Context, key string) (string, error)

	// SetConfig writes a VCS config value.
	SetConfig(ctx context.Context, key, value string) error

	// --- Remote Operations ---

	// GetRemoteURL returns the URL for a named remote.
	GetRemoteURL(ctx context.Context, remote string) (string, error)

	// --- File-Level Operations ---

	// CheckoutFile checks out a specific file from a given revision.
	CheckoutFile(ctx context.Context, ref, path string) error

	// Clean removes untracked files from the working copy.
	Clean(ctx context.Context) error

	// --- Phase 2: Sync-branch worktree/workspace operations ---

	// LogBetween returns commits/changes in 'to' that are not in 'from'.
	// Equivalent to 'git log --oneline from..to'.
	LogBetween(ctx context.Context, from, to string) ([]ChangeInfo, error)

	// DiffPath returns the diff of a specific file between two refs.
	// Equivalent to 'git diff from...to -- path'.
	DiffPath(ctx context.Context, from, to, path string) (string, error)

	// HasStagedChanges returns true if there are staged changes ready to commit.
	// For jj, this checks if the working copy has modifications.
	HasStagedChanges(ctx context.Context) (bool, error)

	// StageAndCommit stages specific files and commits them atomically.
	// For jj, this snapshots and describes the current change.
	StageAndCommit(ctx context.Context, paths []string, message string, opts *CommitOptions) error

	// PushWithUpstream pushes a branch with --set-upstream behavior.
	PushWithUpstream(ctx context.Context, remote, branch string) error

	// Rebase rebases the current branch onto the given ref.
	// For jj: jj rebase -d ref. For git: git rebase ref.
	Rebase(ctx context.Context, onto string) error

	// RebaseAbort aborts a rebase in progress.
	// For jj: no-op (jj handles conflicts inline). For git: git rebase --abort.
	RebaseAbort(ctx context.Context) error

	// --- Phase 3: Hook integration operations ---

	// IsFileTracked returns true if the file is tracked by the VCS.
	// For git: git ls-files --error-unmatch. For jj: jj file list.
	IsFileTracked(ctx context.Context, path string) (bool, error)

	// ConfigureHooksPath sets the hooks directory path.
	// For git: git config core.hooksPath. For jj: configure in jj config.
	ConfigureHooksPath(ctx context.Context, path string) error

	// GetHooksPath returns the configured hooks path, or empty if default.
	GetHooksPath(ctx context.Context) (string, error)

	// ConfigureMergeDriver sets up a custom merge driver for JSONL files.
	// For git: configures merge.beads.driver and merge.beads.name.
	// For jj: no-op (jj handles conflicts differently).
	ConfigureMergeDriver(ctx context.Context, driverCmd, driverName string) error

	// --- Stack Navigation ---

	// Next moves to the next (child) change in the stack.
	// For git: checkout to next unpushed commit. For jj: jj next.
	Next(ctx context.Context) (*ChangeInfo, error)

	// Prev moves to the previous (parent) change in the stack.
	// For git: checkout to parent commit. For jj: jj prev.
	Prev(ctx context.Context) (*ChangeInfo, error)

	// --- Extended Workspace Operations ---

	// UpdateStaleWorkspace refreshes a workspace whose working copy is stale.
	// For jj: jj workspace update-stale. For git: git worktree repair.
	UpdateStaleWorkspace(ctx context.Context, name string) error

	// --- Extended Bookmark/Branch Operations ---

	// DeleteBranch deletes a branch (git) or bookmark (jj).
	DeleteBranch(ctx context.Context, name string) error

	// MoveBranch moves a bookmark to the current change (jj) or resets a branch (git).
	MoveBranch(ctx context.Context, name string, to string) error

	// SetBranch sets a bookmark to a specific revision (jj-specific, alias for MoveBranch in git).
	SetBranch(ctx context.Context, name string, to string) error

	// TrackBranch starts tracking a remote bookmark/branch.
	TrackBranch(ctx context.Context, name string, remote string) error

	// UntrackBranch stops tracking a remote bookmark/branch.
	UntrackBranch(ctx context.Context, name string, remote string) error

	// --- File Operations ---

	// TrackFiles explicitly starts tracking files.
	// For git: git add. For jj: jj file track.
	TrackFiles(ctx context.Context, paths ...string) error

	// UntrackFiles stops tracking files without deleting them.
	// For git: git rm --cached. For jj: jj file untrack.
	UntrackFiles(ctx context.Context, paths ...string) error

	// --- Phase 4: Doctor/maintenance operations ---

	// DiffHasChanges returns true if the file at path differs from the given ref.
	// For git: git diff --quiet ref -- path. For jj: jj diff -r ref --stat path.
	DiffHasChanges(ctx context.Context, ref, path string) (bool, error)

	// RevListCount returns the number of commits between two refs.
	// For git: git rev-list --count from..to. For jj: jj log from..to count.
	RevListCount(ctx context.Context, from, to string) (int, error)

	// MergeBase returns the common ancestor of two refs.
	// For git: git merge-base ref1 ref2. For jj: jj log -r 'heads(::ref1 & ::ref2)'.
	MergeBase(ctx context.Context, ref1, ref2 string) (string, error)

	// GetUpstream returns the upstream tracking ref for the current branch.
	// For git: git rev-parse --abbrev-ref @{u}. For jj: remote bookmark tracking.
	GetUpstream(ctx context.Context) (string, error)

	// CheckIgnore returns true if the path is ignored by VCS ignore rules.
	// For git: git check-ignore. For jj: checks .gitignore (jj uses same format).
	CheckIgnore(ctx context.Context, path string) (bool, error)

	// RestoreFile restores a file from the VCS (discards working copy changes).
	// For git: git restore path. For jj: jj restore path.
	RestoreFile(ctx context.Context, path string) error

	// ResetHard resets the working copy to match the given ref.
	// For git: git reset --hard ref. For jj: jj edit ref.
	ResetHard(ctx context.Context, ref string) error

	// ForcePush pushes with force-with-lease semantics.
	// For git: git push --force-with-lease. For jj: jj git push with force.
	ForcePush(ctx context.Context, remote, branch string) error

	// GetCommonDir returns the shared/common VCS directory.
	// For git: git rev-parse --git-common-dir. For jj: .jj directory.
	GetCommonDir(ctx context.Context) (string, error)

	// ListTrackedFiles returns tracked files matching a path prefix.
	// For git: git ls-files path. For jj: jj file list path.
	ListTrackedFiles(ctx context.Context, path string) ([]string, error)
}

// CommitOptions provides additional options for commits.
type CommitOptions struct {
	Author      string // Override author
	NoGPGSign   bool   // Disable GPG signing (git only)
	AllowEmpty  bool   // Allow empty commits
	Amend       bool   // Amend previous commit (git) / squash into parent (jj)
	Paths       []string // Specific paths to commit
}

// Detector provides methods to detect and create VCS instances.
type Detector interface {
	// Detect detects the VCS type for a given path.
	Detect(path string) (VCSType, error)

	// Create creates a VCS instance for the given path.
	Create(path string) (VCS, error)
}

// DetectVCS detects the VCS type for the given path and returns an appropriate instance.
func DetectVCS(path string) (VCS, error) {
	detector := &DefaultDetector{}
	return detector.Create(path)
}

// DefaultDetector implements Detector with standard detection logic.
type DefaultDetector struct{}

// Detect checks for .jj first (jj), then .git (git), preferring jj in colocated repos.
func (d *DefaultDetector) Detect(path string) (VCSType, error) {
	// Implementation in detect.go
	return detectVCSType(path)
}

// Create returns a VCS instance for the given path.
func (d *DefaultDetector) Create(path string) (VCS, error) {
	vcsType, err := d.Detect(path)
	if err != nil {
		return nil, err
	}

	switch vcsType {
	case VCSTypeJujutsu:
		return NewJujutsuVCS(path)
	case VCSTypeGit:
		return NewGitVCS(path)
	default:
		return nil, ErrNoVCSFound
	}
}
