package vcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// JujutsuVCS implements the VCS interface for Jujutsu (jj).
type JujutsuVCS struct {
	repoRoot    string
	isColocated bool
}

// NewJujutsuVCS creates a new Jujutsu VCS instance.
func NewJujutsuVCS(path string) (*JujutsuVCS, error) {
	root, err := GetJJRoot(path)
	if err != nil {
		return nil, err
	}

	colocated, _ := IsColocatedRepo(root)

	return &JujutsuVCS{
		repoRoot:    root,
		isColocated: colocated,
	}, nil
}

// Type returns VCSTypeJujutsu.
func (j *JujutsuVCS) Type() VCSType {
	return VCSTypeJujutsu
}

// RepoRoot returns the repository root directory.
func (j *JujutsuVCS) RepoRoot() string {
	return j.repoRoot
}

// IsColocated returns true if this is a colocated jj+git repo.
func (j *JujutsuVCS) IsColocated() bool {
	return j.isColocated
}

// Command creates an exec.Cmd for running jj commands.
func (j *JujutsuVCS) Command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = j.repoRoot
	cmd.Env = os.Environ()
	return cmd
}

// runJJ executes a jj command and returns stdout.
func (j *JujutsuVCS) runJJ(ctx context.Context, args ...string) (string, error) {
	cmd := j.Command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", &CommandError{
			VCS:     VCSTypeJujutsu,
			Command: "jj",
			Args:    args,
			Stderr:  stderr.String(),
			Err:     err,
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// runJJJSON executes a jj command with JSON output and returns parsed result.
func (j *JujutsuVCS) runJJJSON(ctx context.Context, args ...string) ([]byte, error) {
	// Add --color=never to prevent color codes in output
	fullArgs := append([]string{"--color=never"}, args...)
	cmd := j.Command(ctx, fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, &CommandError{
			VCS:     VCSTypeJujutsu,
			Command: "jj",
			Args:    fullArgs,
			Stderr:  stderr.String(),
			Err:     err,
		}
	}
	return stdout.Bytes(), nil
}

// CurrentBranch returns the current change ID (@ in jj).
// For jj, we return the change ID since branches (bookmarks) work differently.
func (j *JujutsuVCS) CurrentBranch(ctx context.Context) (string, error) {
	// Use template to get change ID
	output, err := j.runJJ(ctx, "log", "-r", "@", "--no-graph", "-T", "change_id.short()")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// CurrentChange returns info about the current change (@).
func (j *JujutsuVCS) CurrentChange(ctx context.Context) (*ChangeInfo, error) {
	// Get change info using templates
	template := `change_id ++ "\n" ++ change_id.short() ++ "\n" ++ description.first_line() ++ "\n" ++ author.name() ++ "\n" ++ author.timestamp()`
	output, err := j.runJJ(ctx, "log", "-r", "@", "--no-graph", "-T", template)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(output, "\n")
	if len(lines) < 5 {
		return nil, &CommandError{VCS: VCSTypeJujutsu, Command: "log", Err: ErrCommandFailed}
	}

	return &ChangeInfo{
		ID:          lines[0],
		ShortID:     lines[1],
		Description: lines[2],
		Author:      lines[3],
		Timestamp:   lines[4],
		IsWorking:   true, // @ is always the working copy
	}, nil
}

// jjStatusEntry represents a file status entry from jj status.
type jjStatusEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// Status returns the working copy status.
func (j *JujutsuVCS) Status(ctx context.Context) ([]StatusEntry, error) {
	// jj status shows file changes - use diff for more detail
	output, err := j.runJJ(ctx, "diff", "--summary")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return []StatusEntry{}, nil
	}

	var entries []StatusEntry
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		// jj diff --summary format: "M path" or "A path" or "D path"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		statusChar := parts[0]
		path := strings.TrimSpace(parts[1])

		entry := StatusEntry{
			Path:   path,
			Staged: true, // jj auto-snapshots everything
		}

		switch statusChar {
		case "M":
			entry.Status = FileStatusModified
		case "A":
			entry.Status = FileStatusAdded
		case "D":
			entry.Status = FileStatusDeleted
		case "R":
			entry.Status = FileStatusRenamed
		case "C":
			entry.Status = FileStatusCopied
		default:
			entry.Status = FileStatusModified
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// StatusPath returns the status of a specific path.
func (j *JujutsuVCS) StatusPath(ctx context.Context, path string) (*StatusEntry, error) {
	output, err := j.runJJ(ctx, "diff", "--summary", path)
	if err != nil {
		// Path might not exist or no changes
		return &StatusEntry{
			Path:   path,
			Status: FileStatusUnmodified,
		}, nil
	}

	if strings.TrimSpace(output) == "" {
		return &StatusEntry{
			Path:   path,
			Status: FileStatusUnmodified,
		}, nil
	}

	// Parse first line
	line := strings.Split(output, "\n")[0]
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		return &StatusEntry{Path: path, Status: FileStatusUnmodified}, nil
	}

	entry := &StatusEntry{
		Path:   path,
		Staged: true,
	}

	switch parts[0] {
	case "M":
		entry.Status = FileStatusModified
	case "A":
		entry.Status = FileStatusAdded
	case "D":
		entry.Status = FileStatusDeleted
	default:
		entry.Status = FileStatusModified
	}

	return entry, nil
}

// HasRemote returns true if a remote is configured.
func (j *JujutsuVCS) HasRemote(ctx context.Context) (bool, error) {
	// jj git remote list
	output, err := j.runJJ(ctx, "git", "remote", "list")
	if err != nil {
		// Might not be a git-backed repo
		return false, nil
	}
	return strings.TrimSpace(output) != "", nil
}

// GetRemote returns the default remote name.
func (j *JujutsuVCS) GetRemote(ctx context.Context) (string, error) {
	output, err := j.runJJ(ctx, "git", "remote", "list")
	if err != nil {
		return "", ErrNoRemote
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			// Prefer "origin"
			if parts[0] == "origin" {
				return "origin", nil
			}
		}
	}

	// Return first remote if no origin
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			return parts[0], nil
		}
	}

	return "", ErrNoRemote
}

// Stage is a no-op for jj since it auto-snapshots the working copy.
func (j *JujutsuVCS) Stage(ctx context.Context, paths ...string) error {
	// jj doesn't have staging - it auto-snapshots
	// We can use "jj file track" for untracked files if needed
	if len(paths) == 0 {
		return nil
	}

	// Check if any paths are untracked and need tracking
	for _, path := range paths {
		fullPath := filepath.Join(j.repoRoot, path)
		if _, err := os.Stat(fullPath); err == nil {
			// File exists, jj will auto-snapshot it
			// Use file track to ensure it's tracked
			j.runJJ(ctx, "file", "track", path)
		}
	}
	return nil
}

// Commit creates a new change and starts a fresh working copy.
// In jj, this is "jj commit" which creates a new change from @ and starts new @.
func (j *JujutsuVCS) Commit(ctx context.Context, message string, opts *CommitOptions) error {
	args := []string{"commit", "-m", message}

	if opts != nil && opts.Amend {
		// For amend, use squash instead
		return j.Squash(ctx, "")
	}

	_, err := j.runJJ(ctx, args...)
	return err
}

// Fetch fetches from the remote without merging.
func (j *JujutsuVCS) Fetch(ctx context.Context, remote, branch string) error {
	args := []string{"git", "fetch"}
	if remote != "" {
		args = append(args, "--remote", remote)
	}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// Pull fetches from the remote. jj doesn't have "pull" - use fetch.
func (j *JujutsuVCS) Pull(ctx context.Context, remote, branch string) error {
	// jj git fetch does the equivalent of git pull
	return j.Fetch(ctx, remote, branch)
}

// Push pushes to the remote.
func (j *JujutsuVCS) Push(ctx context.Context, remote, branch string) error {
	args := []string{"git", "push"}
	if remote != "" {
		args = append(args, "--remote", remote)
	}
	if branch != "" {
		args = append(args, "--bookmark", branch)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// ListBranches lists all bookmarks (jj's equivalent of branches).
func (j *JujutsuVCS) ListBranches(ctx context.Context) ([]BranchInfo, error) {
	output, err := j.runJJ(ctx, "bookmark", "list", "--all")
	if err != nil {
		return nil, err
	}

	var branches []BranchInfo
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		// Parse bookmark line: "name: change_id description"
		// or "name@remote: change_id description"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 1 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		branch := BranchInfo{Name: name}

		// Check for remote bookmark
		if strings.Contains(name, "@") {
			atParts := strings.SplitN(name, "@", 2)
			branch.Name = atParts[0]
			branch.RemoteName = atParts[1]
		}

		branches = append(branches, branch)
	}

	return branches, nil
}

// CreateBranch creates a new bookmark.
func (j *JujutsuVCS) CreateBranch(ctx context.Context, name string) error {
	_, err := j.runJJ(ctx, "bookmark", "create", name)
	return err
}

// SwitchBranch edits a change that has the given bookmark.
func (j *JujutsuVCS) SwitchBranch(ctx context.Context, name string) error {
	// First try to find the change with this bookmark
	_, err := j.runJJ(ctx, "edit", name)
	return err
}

// ListWorkspaces lists all jj workspaces.
func (j *JujutsuVCS) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	output, err := j.runJJ(ctx, "workspace", "list")
	if err != nil {
		return nil, err
	}

	var workspaces []WorkspaceInfo
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		// Parse workspace line: "name: path @change_id"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		ws := WorkspaceInfo{
			Name: strings.TrimSpace(parts[0]),
		}

		rest := strings.TrimSpace(parts[1])
		// Extract path and change ID
		if atIdx := strings.Index(rest, " @"); atIdx >= 0 {
			ws.Path = strings.TrimSpace(rest[:atIdx])
			ws.ChangeID = strings.TrimSpace(rest[atIdx+2:])
		} else {
			ws.Path = rest
		}

		workspaces = append(workspaces, ws)
	}

	return workspaces, nil
}

// CreateWorkspace creates a new jj workspace.
func (j *JujutsuVCS) CreateWorkspace(ctx context.Context, name, path string) error {
	_, err := j.runJJ(ctx, "workspace", "add", "--name", name, path)
	return err
}

// RemoveWorkspace removes a jj workspace.
func (j *JujutsuVCS) RemoveWorkspace(ctx context.Context, name string) error {
	_, err := j.runJJ(ctx, "workspace", "forget", name)
	return err
}

// HasMergeConflicts returns true if there are unresolved conflicts.
func (j *JujutsuVCS) HasMergeConflicts(ctx context.Context) (bool, error) {
	// Check if @ has conflicts
	output, err := j.runJJ(ctx, "log", "-r", "@", "--no-graph", "-T", "conflict")
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "true"), nil
}

// GetConflicts returns information about conflicts.
func (j *JujutsuVCS) GetConflicts(ctx context.Context) ([]MergeConflict, error) {
	// jj resolve --list shows conflicted files
	output, err := j.runJJ(ctx, "resolve", "--list")
	if err != nil {
		return nil, err
	}

	var conflicts []MergeConflict
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		// Lines are just file paths
		conflicts = append(conflicts, MergeConflict{Path: strings.TrimSpace(line)})
	}

	return conflicts, nil
}

// GetFileVersion retrieves a specific version of a file.
func (j *JujutsuVCS) GetFileVersion(ctx context.Context, path string, version string) ([]byte, error) {
	// For jj, version is a revision specifier
	// Use jj file show
	output, err := j.runJJJSON(ctx, "file", "show", "-r", version, path)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// MarkResolved marks a file as resolved.
func (j *JujutsuVCS) MarkResolved(ctx context.Context, path string) error {
	// jj resolve without arguments resolves conflicts
	_, err := j.runJJ(ctx, "resolve", path)
	return err
}

// Log returns recent changes.
func (j *JujutsuVCS) Log(ctx context.Context, limit int) ([]ChangeInfo, error) {
	// Use template to get structured output
	// Note: Use self.contained_in("@") to check if this is the working copy
	template := `change_id ++ "\x00" ++ change_id.short() ++ "\x00" ++ description.first_line() ++ "\x00" ++ author.name() ++ "\x00" ++ author.timestamp() ++ "\x00" ++ if(self.contained_in("@"), "true", "false") ++ "\n"`

	args := []string{"log", "--no-graph", "-T", template}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	output, err := j.runJJ(ctx, args...)
	if err != nil {
		return nil, err
	}

	var changes []ChangeInfo
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 6 {
			continue
		}
		changes = append(changes, ChangeInfo{
			ID:          parts[0],
			ShortID:     parts[1],
			Description: parts[2],
			Author:      parts[3],
			Timestamp:   parts[4],
			IsWorking:   parts[5] == "true",
		})
	}

	return changes, nil
}

// Show returns details of a specific change.
func (j *JujutsuVCS) Show(ctx context.Context, id string) (*ChangeInfo, error) {
	template := `change_id ++ "\n" ++ change_id.short() ++ "\n" ++ description.first_line() ++ "\n" ++ author.name() ++ "\n" ++ author.timestamp() ++ "\n" ++ if(self.contained_in("@"), "true", "false")`
	output, err := j.runJJ(ctx, "log", "-r", id, "--no-graph", "-T", template)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(output, "\n")
	if len(lines) < 6 {
		return nil, &CommandError{VCS: VCSTypeJujutsu, Command: "log", Err: ErrCommandFailed}
	}

	return &ChangeInfo{
		ID:          lines[0],
		ShortID:     lines[1],
		Description: lines[2],
		Author:      lines[3],
		Timestamp:   lines[4],
		IsWorking:   lines[5] == "true",
	}, nil
}

// Diff returns the diff between two revisions.
func (j *JujutsuVCS) Diff(ctx context.Context, from, to string) (string, error) {
	args := []string{"diff"}
	if from != "" {
		args = append(args, "--from", from)
	}
	if to != "" {
		args = append(args, "--to", to)
	}
	return j.runJJ(ctx, args...)
}

// StackInfo returns the current change stack (mutable changes).
// This is one of jj's unique features - the stack of changes being worked on.
func (j *JujutsuVCS) StackInfo(ctx context.Context) ([]ChangeInfo, error) {
	// Use revset to get the stack: mutable() is changes that can be modified
	// Or use @:: for descendants of current change
	// Let's show the immutable root to @ path
	template := `change_id ++ "\x00" ++ change_id.short() ++ "\x00" ++ description.first_line() ++ "\x00" ++ author.name() ++ "\x00" ++ author.timestamp() ++ "\x00" ++ if(self.contained_in("@"), "true", "false") ++ "\n"`

	// Get mutable changes (the current stack being worked on)
	output, err := j.runJJ(ctx, "log", "-r", "mutable()", "--no-graph", "-T", template)
	if err != nil {
		// Fallback to just @ and parents
		output, err = j.runJJ(ctx, "log", "-r", "::@", "--no-graph", "-T", template, "-n", "10")
		if err != nil {
			return nil, err
		}
	}

	var changes []ChangeInfo
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 6 {
			continue
		}
		changes = append(changes, ChangeInfo{
			ID:          parts[0],
			ShortID:     parts[1],
			Description: parts[2],
			Author:      parts[3],
			Timestamp:   parts[4],
			IsWorking:   parts[5] == "true",
		})
	}

	return changes, nil
}

// Squash squashes the current change into its parent.
func (j *JujutsuVCS) Squash(ctx context.Context, sourceID string) error {
	args := []string{"squash"}
	if sourceID != "" {
		args = append(args, "--from", sourceID)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// New creates a new change on top of the current one.
func (j *JujutsuVCS) New(ctx context.Context, message string) error {
	args := []string{"new"}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// Edit sets a change as the working copy target.
func (j *JujutsuVCS) Edit(ctx context.Context, id string) error {
	_, err := j.runJJ(ctx, "edit", id)
	return err
}

// --- Ref Resolution & Branch Queries ---

// BranchExists returns true if the named bookmark exists.
func (j *JujutsuVCS) BranchExists(ctx context.Context, name string) (bool, error) {
	output, err := j.runJJ(ctx, "bookmark", "list", "--all")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(output, "\n") {
		// jj bookmark list format: "name: changeID description"
		if strings.HasPrefix(line, name+":") || strings.HasPrefix(line, name+" ") {
			return true, nil
		}
	}
	return false, nil
}

// ResolveRef resolves a revision expression to a change ID.
func (j *JujutsuVCS) ResolveRef(ctx context.Context, ref string) (string, error) {
	output, err := j.runJJ(ctx, "log", "--no-graph", "-r", ref,
		"-T", `change_id ++ "\n"`, "--limit", "1")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// IsAncestor returns true if ancestor is an ancestor of descendant.
func (j *JujutsuVCS) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	// Use jj revset: ancestor is in ancestors(descendant)
	output, err := j.runJJ(ctx, "log", "--no-graph", "-r",
		ancestor+" & ancestors("+descendant+")",
		"-T", `change_id`, "--limit", "1")
	if err != nil {
		// If the revset is empty, jj may still succeed with no output
		return false, nil
	}
	return strings.TrimSpace(output) != "", nil
}

// --- Merge Operations ---

// Merge merges the named change. For jj, this creates a merge commit.
func (j *JujutsuVCS) Merge(ctx context.Context, branch, message string) error {
	args := []string{"new", "@", branch}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// IsMerging returns true if a merge is in progress.
// jj doesn't have a "merge in progress" state - merges are atomic.
func (j *JujutsuVCS) IsMerging(ctx context.Context) (bool, error) {
	return false, nil
}

// --- Configuration ---

// GetConfig reads a jj config value.
func (j *JujutsuVCS) GetConfig(ctx context.Context, key string) (string, error) {
	return j.runJJ(ctx, "config", "get", key)
}

// SetConfig writes a jj config value (repo-level).
func (j *JujutsuVCS) SetConfig(ctx context.Context, key, value string) error {
	_, err := j.runJJ(ctx, "config", "set", "--repo", key, value)
	return err
}

// --- Remote Operations ---

// GetRemoteURL returns the URL for a named remote.
func (j *JujutsuVCS) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	// jj stores remote config in .jj/repo/config - try via jj config
	key := "git.remotes." + remote + ".url"
	url, err := j.runJJ(ctx, "config", "get", key)
	if err != nil {
		// Fallback: if colocated, try git
		if j.isColocated {
			cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remote)
			cmd.Dir = j.repoRoot
			out, gitErr := cmd.Output()
			if gitErr == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
		return "", err
	}
	return url, nil
}

// --- File-Level Operations ---

// CheckoutFile checks out a specific file from a given revision.
func (j *JujutsuVCS) CheckoutFile(ctx context.Context, ref, path string) error {
	// jj file show outputs file content; write it to the working copy
	output, err := j.runJJ(ctx, "file", "show", "-r", ref, path)
	if err != nil {
		return err
	}
	fullPath := filepath.Join(j.repoRoot, path)
	return os.WriteFile(fullPath, []byte(output), 0644)
}

// Clean is a no-op for jj - jj tracks all files automatically.
func (j *JujutsuVCS) Clean(ctx context.Context) error {
	return nil
}

// --- Stack Navigation ---

// Next moves to the next (child) change in the stack.
func (j *JujutsuVCS) Next(ctx context.Context) (*ChangeInfo, error) {
	_, err := j.runJJ(ctx, "next")
	if err != nil {
		return nil, err
	}
	return j.CurrentChange(ctx)
}

// Prev moves to the previous (parent) change in the stack.
func (j *JujutsuVCS) Prev(ctx context.Context) (*ChangeInfo, error) {
	_, err := j.runJJ(ctx, "prev")
	if err != nil {
		return nil, err
	}
	return j.CurrentChange(ctx)
}

// --- Extended Workspace Operations ---

// UpdateStaleWorkspace refreshes a workspace whose working copy is stale.
func (j *JujutsuVCS) UpdateStaleWorkspace(ctx context.Context, name string) error {
	if name == "" || name == "default" {
		_, err := j.runJJ(ctx, "workspace", "update-stale")
		return err
	}
	// For a specific workspace, run update-stale from that workspace's directory
	workspaces, err := j.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	for _, ws := range workspaces {
		if ws.Name == name {
			// Check if workspace path exists
			if _, statErr := os.Stat(ws.Path); statErr != nil {
				// Workspace path doesn't exist, run from repo root
				_, err := j.runJJ(ctx, "workspace", "update-stale")
				return err
			}
			cmd := exec.CommandContext(ctx, "jj", "workspace", "update-stale")
			cmd.Dir = ws.Path
			cmd.Env = os.Environ()
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				return &CommandError{
					VCS:     VCSTypeJujutsu,
					Command: "jj",
					Args:    []string{"workspace", "update-stale"},
					Stderr:  stderr.String(),
					Err:     err,
				}
			}
			return nil
		}
	}
	return ErrWorkspaceNotFound
}

// --- Extended Bookmark Operations ---

// DeleteBranch deletes a bookmark.
func (j *JujutsuVCS) DeleteBranch(ctx context.Context, name string) error {
	_, err := j.runJJ(ctx, "bookmark", "delete", name)
	return err
}

// MoveBranch moves a bookmark to the specified revision.
func (j *JujutsuVCS) MoveBranch(ctx context.Context, name string, to string) error {
	args := []string{"bookmark", "move", name}
	if to != "" {
		args = append(args, "--to", to)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// SetBranch sets a bookmark to a specific revision.
func (j *JujutsuVCS) SetBranch(ctx context.Context, name string, to string) error {
	args := []string{"bookmark", "set", name}
	if to != "" {
		args = append(args, "-r", to)
	}
	_, err := j.runJJ(ctx, args...)
	return err
}

// TrackBranch starts tracking a remote bookmark.
func (j *JujutsuVCS) TrackBranch(ctx context.Context, name string, remote string) error {
	ref := name
	if remote != "" {
		ref = name + "@" + remote
	}
	_, err := j.runJJ(ctx, "bookmark", "track", ref)
	return err
}

// UntrackBranch stops tracking a remote bookmark.
func (j *JujutsuVCS) UntrackBranch(ctx context.Context, name string, remote string) error {
	ref := name
	if remote != "" {
		ref = name + "@" + remote
	}
	_, err := j.runJJ(ctx, "bookmark", "untrack", ref)
	return err
}

// --- File Operations ---

// TrackFiles explicitly starts tracking files.
func (j *JujutsuVCS) TrackFiles(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"file", "track"}, paths...)
	_, err := j.runJJ(ctx, args...)
	return err
}

// UntrackFiles stops tracking files without deleting them.
func (j *JujutsuVCS) UntrackFiles(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"file", "untrack"}, paths...)
	_, err := j.runJJ(ctx, args...)
	return err
}

// --- Phase 2: Sync-branch worktree/workspace operations ---

// LogBetween returns changes in 'to' that are not in 'from'.
// Uses jj revset: 'to ~ from' (commits in to but not in from).
func (j *JujutsuVCS) LogBetween(ctx context.Context, from, to string) ([]ChangeInfo, error) {
	revset := fmt.Sprintf("(%s) ~ (%s)", to, from)
	output, err := j.runJJ(ctx, "log", "--no-graph", "-r", revset,
		"-T", `change_id ++ "\x00" ++ change_id.shortest(8) ++ "\x00" ++ description.first_line() ++ "\x00" ++ author.name() ++ "\x00" ++ author.timestamp() ++ "\n"`)
	if err != nil {
		return nil, err
	}

	var changes []ChangeInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 6)
		if len(parts) < 5 {
			continue
		}
		changes = append(changes, ChangeInfo{
			ID:          strings.TrimSpace(parts[0]),
			ShortID:     strings.TrimSpace(parts[1]),
			Description: strings.TrimSpace(parts[2]),
			Author:      strings.TrimSpace(parts[3]),
			Timestamp:   strings.TrimSpace(parts[4]),
		})
	}
	return changes, nil
}

// DiffPath returns the diff of a specific file between two refs.
func (j *JujutsuVCS) DiffPath(ctx context.Context, from, to, path string) (string, error) {
	args := []string{"diff", "--from", from, "--to", to}
	if path != "" {
		args = append(args, path)
	}
	return j.runJJ(ctx, args...)
}

// HasStagedChanges returns true if the working copy has modifications.
// jj auto-tracks everything, so check if the working copy change is non-empty.
func (j *JujutsuVCS) HasStagedChanges(ctx context.Context) (bool, error) {
	output, err := j.runJJ(ctx, "diff", "--stat")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// StageAndCommit snapshots and describes the current change, then creates a new one.
func (j *JujutsuVCS) StageAndCommit(ctx context.Context, paths []string, message string, opts *CommitOptions) error {
	// jj auto-snapshots, so just describe and commit
	_, err := j.runJJ(ctx, "commit", "-m", message)
	return err
}

// PushWithUpstream pushes a bookmark with tracking setup.
func (j *JujutsuVCS) PushWithUpstream(ctx context.Context, remote, branch string) error {
	_, err := j.runJJ(ctx, "git", "push", "--remote", remote, "--bookmark", branch)
	return err
}

// Rebase rebases the current change onto the given ref (interface method).
func (j *JujutsuVCS) Rebase(ctx context.Context, onto string) error {
	_, err := j.runJJ(ctx, "rebase", "-d", onto)
	return err
}

// RebaseAbort is a no-op for jj (jj handles conflicts inline, no "rebase in progress" state).
func (j *JujutsuVCS) RebaseAbort(ctx context.Context) error {
	return nil // jj doesn't have an "in-progress rebase" state
}

// --- Phase 3: Hook integration operations ---

// IsFileTracked returns true if the file is tracked by jj.
func (j *JujutsuVCS) IsFileTracked(ctx context.Context, path string) (bool, error) {
	output, err := j.runJJ(ctx, "file", "list", path)
	if err != nil {
		return false, nil // Error means not tracked or jj issue
	}
	return strings.TrimSpace(output) != "", nil
}

// ConfigureHooksPath sets the hooks directory for jj.
// jj doesn't have a native hooks path config - uses .beads/jj-hooks.toml instead.
func (j *JujutsuVCS) ConfigureHooksPath(ctx context.Context, path string) error {
	// jj hooks are managed by .beads/jj-hooks.toml and wrapper scripts
	// No native jj config equivalent - this is a no-op.
	// The hooks_jj.go file handles jj hook installation separately.
	return nil
}

// GetHooksPath returns the hooks path for jj.
// jj doesn't use core.hooksPath - returns empty.
func (j *JujutsuVCS) GetHooksPath(ctx context.Context) (string, error) {
	return "", nil // jj manages hooks differently
}

// ConfigureMergeDriver configures a merge driver for jj.
// jj handles conflicts inline (not via merge drivers), so this is a no-op.
func (j *JujutsuVCS) ConfigureMergeDriver(ctx context.Context, driverCmd, driverName string) error {
	return nil // jj handles conflicts differently
}

// --- jj-specific helper methods not in VCS interface ---

// Describe updates the description of the current change.
func (j *JujutsuVCS) Describe(ctx context.Context, message string) error {
	_, err := j.runJJ(ctx, "describe", "-m", message)
	return err
}

// RebaseRevision rebases a specific revision onto a new base (jj-specific).
func (j *JujutsuVCS) RebaseRevision(ctx context.Context, source, destination string) error {
	args := []string{"rebase"}
	if source != "" {
		args = append(args, "-r", source)
	}
	args = append(args, "-d", destination)
	_, err := j.runJJ(ctx, args...)
	return err
}

// Abandon abandons changes (marks them as hidden).
func (j *JujutsuVCS) Abandon(ctx context.Context, revisions ...string) error {
	args := []string{"abandon"}
	args = append(args, revisions...)
	_, err := j.runJJ(ctx, args...)
	return err
}

// GitExport exports jj changes to git (for colocated repos).
func (j *JujutsuVCS) GitExport(ctx context.Context) error {
	_, err := j.runJJ(ctx, "git", "export")
	return err
}

// GitImport imports git changes into jj (for colocated repos).
func (j *JujutsuVCS) GitImport(ctx context.Context) error {
	_, err := j.runJJ(ctx, "git", "import")
	return err
}

// Snapshot forces a snapshot of the working copy.
func (j *JujutsuVCS) Snapshot(ctx context.Context) error {
	// jj status triggers a snapshot
	_, err := j.runJJ(ctx, "status")
	return err
}

// Ensure JujutsuVCS implements VCS.
var _ VCS = (*JujutsuVCS)(nil)

// Additional helper for JSON parsing if needed
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
