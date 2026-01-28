package vcs

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// GitVCS implements the VCS interface for Git.
type GitVCS struct {
	repoRoot   string
	isColocated bool
}

// NewGitVCS creates a new Git VCS instance.
func NewGitVCS(path string) (*GitVCS, error) {
	root, err := GetGitRoot(path)
	if err != nil {
		return nil, err
	}

	colocated, _ := IsColocatedRepo(root)

	return &GitVCS{
		repoRoot:   root,
		isColocated: colocated,
	}, nil
}

// Type returns VCSTypeGit.
func (g *GitVCS) Type() VCSType {
	return VCSTypeGit
}

// RepoRoot returns the repository root directory.
func (g *GitVCS) RepoRoot() string {
	return g.repoRoot
}

// IsColocated returns true if this is a colocated jj+git repo.
func (g *GitVCS) IsColocated() bool {
	return g.isColocated
}

// Command creates an exec.Cmd for running git commands.
func (g *GitVCS) Command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repoRoot
	// Security: Disable hooks and templates to prevent unexpected execution
	cmd.Env = append(os.Environ(),
		"GIT_HOOKS_PATH=",
		"GIT_TEMPLATE_DIR=",
	)
	return cmd
}

// runGit executes a git command and returns stdout.
func (g *GitVCS) runGit(ctx context.Context, args ...string) (string, error) {
	cmd := g.Command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", &CommandError{
			VCS:     VCSTypeGit,
			Command: "git",
			Args:    args,
			Stderr:  stderr.String(),
			Err:     err,
		}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CurrentBranch returns the current branch name.
func (g *GitVCS) CurrentBranch(ctx context.Context) (string, error) {
	return g.runGit(ctx, "symbolic-ref", "--short", "HEAD")
}

// CurrentChange returns info about the current commit.
func (g *GitVCS) CurrentChange(ctx context.Context) (*ChangeInfo, error) {
	// Get commit info with a single log command
	format := "%H%n%h%n%s%n%an%n%ai"
	output, err := g.runGit(ctx, "log", "-1", "--format="+format)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(output, "\n")
	if len(lines) < 5 {
		return nil, &CommandError{VCS: VCSTypeGit, Command: "log", Err: ErrCommandFailed}
	}

	return &ChangeInfo{
		ID:          lines[0],
		ShortID:     lines[1],
		Description: lines[2],
		Author:      lines[3],
		Timestamp:   lines[4],
		IsWorking:   false,
	}, nil
}

// Status returns the working copy status.
func (g *GitVCS) Status(ctx context.Context) ([]StatusEntry, error) {
	output, err := g.runGit(ctx, "status", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return []StatusEntry{}, nil
	}

	var entries []StatusEntry
	// Parse NUL-delimited output
	parts := strings.Split(output, "\x00")
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if len(part) < 3 {
			continue
		}

		indexStatus := part[0]
		worktreeStatus := part[1]
		path := part[3:]

		entry := StatusEntry{
			Path: path,
		}

		// Determine status from index and worktree status
		switch {
		case indexStatus == 'U' || worktreeStatus == 'U':
			entry.Status = FileStatusConflicted
			entry.Conflicted = true
		case indexStatus == 'A':
			entry.Status = FileStatusAdded
			entry.Staged = true
		case indexStatus == 'D' || worktreeStatus == 'D':
			entry.Status = FileStatusDeleted
			entry.Staged = indexStatus == 'D'
		case indexStatus == 'M' || worktreeStatus == 'M':
			entry.Status = FileStatusModified
			entry.Staged = indexStatus == 'M'
		case indexStatus == 'R':
			entry.Status = FileStatusRenamed
			entry.Staged = true
			// Next part is the old path for renames
			if i+1 < len(parts) {
				entry.OldPath = parts[i+1]
				i++
			}
		case indexStatus == 'C':
			entry.Status = FileStatusCopied
			entry.Staged = true
			if i+1 < len(parts) {
				entry.OldPath = parts[i+1]
				i++
			}
		case indexStatus == '?' && worktreeStatus == '?':
			entry.Status = FileStatusUntracked
		case indexStatus == '!' && worktreeStatus == '!':
			entry.Status = FileStatusIgnored
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// StatusPath returns the status of a specific path.
func (g *GitVCS) StatusPath(ctx context.Context, path string) (*StatusEntry, error) {
	output, err := g.runGit(ctx, "status", "--porcelain", path)
	if err != nil {
		return nil, err
	}

	if output == "" {
		return &StatusEntry{
			Path:   path,
			Status: FileStatusUnmodified,
		}, nil
	}

	// Parse first line of porcelain output
	if len(output) < 3 {
		return nil, &CommandError{VCS: VCSTypeGit, Command: "status", Err: ErrCommandFailed}
	}

	entry := &StatusEntry{
		Path: path,
	}

	indexStatus := output[0]
	worktreeStatus := output[1]

	switch {
	case indexStatus == 'U' || worktreeStatus == 'U':
		entry.Status = FileStatusConflicted
		entry.Conflicted = true
	case indexStatus == 'A':
		entry.Status = FileStatusAdded
		entry.Staged = true
	case indexStatus == 'D' || worktreeStatus == 'D':
		entry.Status = FileStatusDeleted
		entry.Staged = indexStatus == 'D'
	case indexStatus == 'M' || worktreeStatus == 'M':
		entry.Status = FileStatusModified
		entry.Staged = indexStatus == 'M'
	case indexStatus == '?':
		entry.Status = FileStatusUntracked
	case indexStatus == '!':
		entry.Status = FileStatusIgnored
	default:
		entry.Status = FileStatusModified
	}

	return entry, nil
}

// HasRemote returns true if a remote is configured.
func (g *GitVCS) HasRemote(ctx context.Context) (bool, error) {
	output, err := g.runGit(ctx, "remote")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// GetRemote returns the default remote name.
func (g *GitVCS) GetRemote(ctx context.Context) (string, error) {
	output, err := g.runGit(ctx, "remote")
	if err != nil {
		return "", err
	}

	remotes := strings.Fields(output)
	if len(remotes) == 0 {
		return "", ErrNoRemote
	}

	// Prefer "origin" if it exists
	for _, r := range remotes {
		if r == "origin" {
			return "origin", nil
		}
	}
	return remotes[0], nil
}

// Stage stages files for the next commit.
func (g *GitVCS) Stage(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add"}, paths...)
	_, err := g.runGit(ctx, args...)
	return err
}

// Commit creates a new commit with the given message.
func (g *GitVCS) Commit(ctx context.Context, message string, opts *CommitOptions) error {
	args := []string{"commit", "-m", message}

	if opts != nil {
		if opts.Author != "" {
			args = append(args, "--author", opts.Author)
		}
		if opts.NoGPGSign {
			args = append(args, "--no-gpg-sign")
		}
		if opts.AllowEmpty {
			args = append(args, "--allow-empty")
		}
		if opts.Amend {
			args = append(args, "--amend")
		}
		if len(opts.Paths) > 0 {
			args = append(args, "--")
			args = append(args, opts.Paths...)
		}
	}

	_, err := g.runGit(ctx, args...)
	return err
}

// Fetch fetches from the remote without merging.
func (g *GitVCS) Fetch(ctx context.Context, remote, branch string) error {
	args := []string{"fetch"}
	if remote != "" {
		args = append(args, remote)
	}
	if branch != "" {
		args = append(args, branch)
	}
	_, err := g.runGit(ctx, args...)
	return err
}

// Pull fetches and merges from the remote.
func (g *GitVCS) Pull(ctx context.Context, remote, branch string) error {
	args := []string{"pull"}
	if remote != "" {
		args = append(args, remote)
	}
	if branch != "" {
		args = append(args, branch)
	}
	_, err := g.runGit(ctx, args...)
	return err
}

// Push pushes to the remote.
func (g *GitVCS) Push(ctx context.Context, remote, branch string) error {
	args := []string{"push"}
	if remote != "" {
		args = append(args, remote)
	}
	if branch != "" {
		args = append(args, branch)
	}
	_, err := g.runGit(ctx, args...)
	return err
}

// ListBranches lists all branches.
func (g *GitVCS) ListBranches(ctx context.Context) ([]BranchInfo, error) {
	output, err := g.runGit(ctx, "branch", "-a", "--format=%(refname:short)%(if)%(HEAD)%(then)*%(end)")
	if err != nil {
		return nil, err
	}

	var branches []BranchInfo
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}

		isCurrent := strings.HasSuffix(line, "*")
		name := strings.TrimSuffix(line, "*")

		branch := BranchInfo{
			Name:      name,
			IsCurrent: isCurrent,
		}

		// Check if it's a remote branch
		if strings.HasPrefix(name, "remotes/") {
			parts := strings.SplitN(strings.TrimPrefix(name, "remotes/"), "/", 2)
			if len(parts) == 2 {
				branch.RemoteName = parts[0]
				branch.Name = parts[1]
			}
		}

		branches = append(branches, branch)
	}

	return branches, nil
}

// CreateBranch creates a new branch.
func (g *GitVCS) CreateBranch(ctx context.Context, name string) error {
	_, err := g.runGit(ctx, "branch", name)
	return err
}

// SwitchBranch switches to a different branch.
func (g *GitVCS) SwitchBranch(ctx context.Context, name string) error {
	_, err := g.runGit(ctx, "checkout", name)
	return err
}

// ListWorkspaces lists all worktrees.
func (g *GitVCS) ListWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	output, err := g.runGit(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var workspaces []WorkspaceInfo
	var current *WorkspaceInfo

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current != nil {
				workspaces = append(workspaces, *current)
			}
			current = &WorkspaceInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "HEAD ") && current != nil {
			current.ChangeID = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			current.Name = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}

	if current != nil {
		workspaces = append(workspaces, *current)
	}

	return workspaces, nil
}

// CreateWorkspace creates a new worktree.
func (g *GitVCS) CreateWorkspace(ctx context.Context, name, path string) error {
	_, err := g.runGit(ctx, "worktree", "add", "-b", name, path)
	return err
}

// RemoveWorkspace removes a worktree.
func (g *GitVCS) RemoveWorkspace(ctx context.Context, name string) error {
	// First find the path for the worktree
	workspaces, err := g.ListWorkspaces(ctx)
	if err != nil {
		return err
	}

	var path string
	for _, ws := range workspaces {
		if ws.Name == name {
			path = ws.Path
			break
		}
	}

	if path == "" {
		return ErrWorkspaceNotFound
	}

	_, err = g.runGit(ctx, "worktree", "remove", path, "--force")
	return err
}

// HasMergeConflicts returns true if there are unresolved merge conflicts.
func (g *GitVCS) HasMergeConflicts(ctx context.Context) (bool, error) {
	output, err := g.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	// Look for unmerged entries (UU, AA, DD, etc.)
	for _, line := range strings.Split(output, "\n") {
		if len(line) >= 2 {
			if line[0] == 'U' || line[1] == 'U' {
				return true, nil
			}
		}
	}
	return false, nil
}

// GetConflicts returns information about merge conflicts.
func (g *GitVCS) GetConflicts(ctx context.Context) ([]MergeConflict, error) {
	output, err := g.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return nil, err
	}

	var conflicts []MergeConflict
	for _, line := range strings.Split(output, "\n") {
		if len(line) >= 4 && (line[0] == 'U' || line[1] == 'U') {
			path := strings.TrimSpace(line[3:])
			conflicts = append(conflicts, MergeConflict{Path: path})
		}
	}
	return conflicts, nil
}

// GetFileVersion retrieves a specific version of a file.
func (g *GitVCS) GetFileVersion(ctx context.Context, path string, version string) ([]byte, error) {
	// version can be: "base" (stage 1), "ours" (stage 2), "theirs" (stage 3), or a ref
	var spec string
	switch version {
	case "base", "1":
		spec = ":1:" + path
	case "ours", "2":
		spec = ":2:" + path
	case "theirs", "3":
		spec = ":3:" + path
	default:
		spec = version + ":" + path
	}

	cmd := g.Command(ctx, "show", spec)
	output, err := cmd.Output()
	if err != nil {
		return nil, &CommandError{
			VCS:     VCSTypeGit,
			Command: "show",
			Args:    []string{spec},
			Err:     err,
		}
	}
	return output, nil
}

// MarkResolved marks a file as resolved.
func (g *GitVCS) MarkResolved(ctx context.Context, path string) error {
	return g.Stage(ctx, path)
}

// Log returns recent commits.
func (g *GitVCS) Log(ctx context.Context, limit int) ([]ChangeInfo, error) {
	format := "%H%x00%h%x00%s%x00%an%x00%ai%x00"
	args := []string{"log", "--format=" + format}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}

	output, err := g.runGit(ctx, args...)
	if err != nil {
		return nil, err
	}

	var changes []ChangeInfo
	for _, record := range strings.Split(output, "\n") {
		if record == "" {
			continue
		}
		parts := strings.Split(record, "\x00")
		if len(parts) < 5 {
			continue
		}
		changes = append(changes, ChangeInfo{
			ID:          parts[0],
			ShortID:     parts[1],
			Description: parts[2],
			Author:      parts[3],
			Timestamp:   parts[4],
		})
	}

	return changes, nil
}

// Show returns details of a specific commit.
func (g *GitVCS) Show(ctx context.Context, id string) (*ChangeInfo, error) {
	format := "%H%n%h%n%s%n%an%n%ai"
	output, err := g.runGit(ctx, "log", "-1", "--format="+format, id)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(output, "\n")
	if len(lines) < 5 {
		return nil, &CommandError{VCS: VCSTypeGit, Command: "log", Err: ErrCommandFailed}
	}

	return &ChangeInfo{
		ID:          lines[0],
		ShortID:     lines[1],
		Description: lines[2],
		Author:      lines[3],
		Timestamp:   lines[4],
	}, nil
}

// Diff returns the diff between two revisions.
func (g *GitVCS) Diff(ctx context.Context, from, to string) (string, error) {
	args := []string{"diff"}
	if from != "" {
		args = append(args, from)
	}
	if to != "" {
		args = append(args, to)
	}
	return g.runGit(ctx, args...)
}

// StackInfo returns unpushed commits on the current branch.
func (g *GitVCS) StackInfo(ctx context.Context) ([]ChangeInfo, error) {
	// Get unpushed commits
	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	remote, err := g.GetRemote(ctx)
	if err != nil {
		// No remote, return all commits
		return g.Log(ctx, 10)
	}

	// Show commits not in remote
	output, err := g.runGit(ctx, "log", "--format=%H%x00%h%x00%s%x00%an%x00%ai%x00",
		remote+"/"+branch+"..HEAD")
	if err != nil {
		// Remote branch might not exist
		return g.Log(ctx, 10)
	}

	var changes []ChangeInfo
	for _, record := range strings.Split(output, "\n") {
		if record == "" {
			continue
		}
		parts := strings.Split(record, "\x00")
		if len(parts) < 5 {
			continue
		}
		changes = append(changes, ChangeInfo{
			ID:          parts[0],
			ShortID:     parts[1],
			Description: parts[2],
			Author:      parts[3],
			Timestamp:   parts[4],
		})
	}

	return changes, nil
}

// Squash amends the previous commit.
func (g *GitVCS) Squash(ctx context.Context, sourceID string) error {
	// For git, this is essentially commit --amend
	_, err := g.runGit(ctx, "commit", "--amend", "--no-edit")
	return err
}

// New is a no-op for git (commits are created with Commit).
func (g *GitVCS) New(ctx context.Context, message string) error {
	// Git doesn't have jj's "new" concept
	// This could create an empty commit as a placeholder
	return nil
}

// Edit checks out a specific commit.
func (g *GitVCS) Edit(ctx context.Context, id string) error {
	_, err := g.runGit(ctx, "checkout", id)
	return err
}

// --- Stack Navigation ---

// Next moves to the next (child) commit in the stack.
// For git, this walks to a child commit on the current branch.
func (g *GitVCS) Next(ctx context.Context) (*ChangeInfo, error) {
	// Get current HEAD
	head, err := g.runGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}

	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	// Find child commit: commits whose parent is HEAD and are ancestors of branch tip
	output, err := g.runGit(ctx, "log", "--reverse", "--ancestry-path", "--format=%H",
		head+".."+branch)
	if err != nil || output == "" {
		return nil, &CommandError{VCS: VCSTypeGit, Command: "next",
			Err: ErrCommandFailed, Stderr: "no next commit in stack"}
	}

	// Take the first child
	childHash := strings.Split(output, "\n")[0]
	_, err = g.runGit(ctx, "checkout", childHash)
	if err != nil {
		return nil, err
	}

	return g.CurrentChange(ctx)
}

// Prev moves to the previous (parent) commit.
func (g *GitVCS) Prev(ctx context.Context) (*ChangeInfo, error) {
	_, err := g.runGit(ctx, "checkout", "HEAD~1")
	if err != nil {
		return nil, err
	}
	return g.CurrentChange(ctx)
}

// --- Extended Workspace Operations ---

// UpdateStaleWorkspace repairs a stale worktree.
func (g *GitVCS) UpdateStaleWorkspace(ctx context.Context, name string) error {
	_, err := g.runGit(ctx, "worktree", "repair")
	return err
}

// --- Extended Bookmark/Branch Operations ---

// DeleteBranch deletes a branch.
func (g *GitVCS) DeleteBranch(ctx context.Context, name string) error {
	_, err := g.runGit(ctx, "branch", "-d", name)
	return err
}

// MoveBranch moves a branch to a specific commit.
func (g *GitVCS) MoveBranch(ctx context.Context, name string, to string) error {
	if to == "" {
		to = "HEAD"
	}
	_, err := g.runGit(ctx, "branch", "-f", name, to)
	return err
}

// SetBranch sets a branch to a specific commit (same as MoveBranch for git).
func (g *GitVCS) SetBranch(ctx context.Context, name string, to string) error {
	return g.MoveBranch(ctx, name, to)
}

// TrackBranch sets up tracking for a remote branch.
func (g *GitVCS) TrackBranch(ctx context.Context, name string, remote string) error {
	if remote == "" {
		remote = "origin"
	}
	_, err := g.runGit(ctx, "branch", "--set-upstream-to="+remote+"/"+name, name)
	return err
}

// UntrackBranch removes tracking for a remote branch.
func (g *GitVCS) UntrackBranch(ctx context.Context, name string, remote string) error {
	_, err := g.runGit(ctx, "branch", "--unset-upstream", name)
	return err
}

// --- File Operations ---

// TrackFiles starts tracking files (git add).
func (g *GitVCS) TrackFiles(ctx context.Context, paths ...string) error {
	return g.Stage(ctx, paths...)
}

// UntrackFiles stops tracking files without deleting them.
func (g *GitVCS) UntrackFiles(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"rm", "--cached"}, paths...)
	_, err := g.runGit(ctx, args...)
	return err
}

// Ensure GitVCS implements VCS.
var _ VCS = (*GitVCS)(nil)
