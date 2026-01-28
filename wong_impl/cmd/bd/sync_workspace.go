// sync_workspace.go adapts the sync-branch worktree system to use jj workspaces.
//
// The existing sync_branch.go / daemon_sync_branch.go use git worktrees with
// sparse checkout to manage a separate sync branch for JSONL files. This file
// provides the jj workspace equivalent.
//
// Sync workspace path: .jj/beads-sync-workspace (analogous to .git/beads-worktrees/)
//
// Usage:
//   - isJJSyncWorkspace checks whether to use this path vs git worktrees
//   - syncWorkspaceCreateOrGet creates/finds the sync workspace
//   - syncWorkspaceCommitJSONL commits JSONL changes in the workspace
//   - syncWorkspacePull / syncWorkspacePush handle remote sync
//   - syncWorkspaceMergeBack copies JSONL back to the main workspace
//   - syncWorkspaceCleanup removes the sync workspace
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/beads/internal/vcs"
)

// syncWorkspaceDirName is the directory name for the jj sync workspace,
// stored inside the .jj directory (parallel to .git/beads-worktrees/).
const syncWorkspaceDirName = "beads-sync-workspace"

// isJJSyncWorkspace checks if we should use jj workspace sync.
// Returns true if the repo at repoRoot is a jj repository.
func isJJSyncWorkspace(repoRoot string) bool {
	jjDir := filepath.Join(repoRoot, ".jj")
	info, err := os.Stat(jjDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// syncWorkspacePath returns the absolute path for the sync workspace directory.
func syncWorkspacePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".jj", syncWorkspaceDirName)
}

// syncWorkspaceCreateOrGet creates or gets an existing jj workspace for sync operations.
// It returns the absolute path to the workspace directory.
func syncWorkspaceCreateOrGet(ctx context.Context, repoRoot string, syncName string) (string, error) {
	wsPath := syncWorkspacePath(repoRoot)

	// Check if workspace directory already exists
	if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
		// Workspace directory exists; ensure it is not stale
		if err := runJJFromDir(ctx, repoRoot, "workspace", "update-stale"); err != nil {
			// Non-fatal: workspace may not actually be stale
		}
		return wsPath, nil
	}

	// Create the workspace directory parent if needed
	if err := os.MkdirAll(filepath.Dir(wsPath), 0o755); err != nil {
		return "", fmt.Errorf("creating sync workspace parent dir: %w", err)
	}

	// Create the jj workspace
	if err := runJJFromDir(ctx, repoRoot, "workspace", "add", "--name", syncName, wsPath); err != nil {
		return "", fmt.Errorf("creating jj workspace %q at %s: %w", syncName, wsPath, err)
	}

	return wsPath, nil
}

// syncWorkspaceCommitJSONL commits JSONL changes in the sync workspace.
// It snapshots the working copy and creates a commit with the given message.
func syncWorkspaceCommitJSONL(ctx context.Context, wsPath string, jsonlRelPath string, message string) error {
	// Ensure the file is tracked
	if err := runJJFromDir(ctx, wsPath, "file", "track", jsonlRelPath); err != nil {
		// Non-fatal: file may already be tracked
	}

	// Snapshot the working copy (jj status triggers this)
	if err := runJJFromDir(ctx, wsPath, "status"); err != nil {
		return fmt.Errorf("snapshot sync workspace: %w", err)
	}

	// Check if there are actual changes to commit
	diffOut, err := runJJFromDirOutput(ctx, wsPath, "diff", "--summary")
	if err != nil {
		return fmt.Errorf("checking diff in sync workspace: %w", err)
	}
	if strings.TrimSpace(diffOut) == "" {
		// No changes to commit
		return nil
	}

	// Commit the current working copy change and start a new one
	if err := runJJFromDir(ctx, wsPath, "commit", "-m", message); err != nil {
		return fmt.Errorf("committing JSONL in sync workspace: %w", err)
	}

	return nil
}

// syncWorkspacePull pulls latest changes into the sync workspace from the remote.
func syncWorkspacePull(ctx context.Context, wsPath string, remote string) error {
	args := []string{"git", "fetch"}
	if remote != "" {
		args = append(args, "--remote", remote)
	}

	if err := runJJFromDir(ctx, wsPath, args...); err != nil {
		return fmt.Errorf("fetching into sync workspace: %w", err)
	}

	return nil
}

// syncWorkspacePush pushes from the sync workspace to the remote bookmark.
func syncWorkspacePush(ctx context.Context, wsPath string, remote string, bookmark string) error {
	args := []string{"git", "push"}
	if remote != "" {
		args = append(args, "--remote", remote)
	}
	if bookmark != "" {
		args = append(args, "--bookmark", bookmark)
	}

	if err := runJJFromDir(ctx, wsPath, args...); err != nil {
		return fmt.Errorf("pushing from sync workspace: %w", err)
	}

	return nil
}

// syncWorkspaceMergeBack copies the JSONL file from the sync workspace back
// to the main workspace. This is the jj equivalent of merging the sync branch.
func syncWorkspaceMergeBack(ctx context.Context, repoRoot string, wsPath string, jsonlRelPath string) error {
	srcPath := filepath.Join(wsPath, jsonlRelPath)
	dstPath := filepath.Join(repoRoot, jsonlRelPath)

	// Read from sync workspace
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No JSONL in sync workspace yet, nothing to merge back
			return nil
		}
		return fmt.Errorf("reading JSONL from sync workspace: %w", err)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("creating JSONL destination dir: %w", err)
	}

	// Write to main workspace
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		return fmt.Errorf("writing JSONL to main workspace: %w", err)
	}

	return nil
}

// syncWorkspaceCleanup removes the sync workspace entirely.
func syncWorkspaceCleanup(ctx context.Context, repoRoot string, syncName string) error {
	// Forget the workspace in jj first
	if err := runJJFromDir(ctx, repoRoot, "workspace", "forget", syncName); err != nil {
		// Log but continue to clean up the directory
	}

	// Remove the workspace directory
	wsPath := syncWorkspacePath(repoRoot)
	if err := os.RemoveAll(wsPath); err != nil {
		return fmt.Errorf("removing sync workspace directory: %w", err)
	}

	return nil
}

// --- helpers ---

// runJJFromDir runs a jj command from the given directory.
func runJJFromDir(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &vcs.CommandError{
			VCS:     vcs.VCSTypeJujutsu,
			Command: "jj",
			Args:    args,
			Stderr:  stderr.String(),
			Err:     err,
		}
	}
	return nil
}

// runJJFromDirOutput runs a jj command from the given directory and returns stdout.
func runJJFromDirOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &vcs.CommandError{
			VCS:     vcs.VCSTypeJujutsu,
			Command: "jj",
			Args:    args,
			Stderr:  stderr.String(),
			Err:     err,
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}
