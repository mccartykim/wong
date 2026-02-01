package vcs

import "errors"

var (
	// ErrNoVCSFound is returned when no version control system is detected.
	ErrNoVCSFound = errors.New("no version control system found")

	// ErrNotInRepo is returned when the operation requires being in a repository.
	ErrNotInRepo = errors.New("not in a repository")

	// ErrNoRemote is returned when a remote is required but none is configured.
	ErrNoRemote = errors.New("no remote configured")

	// ErrBranchNotFound is returned when a branch/bookmark doesn't exist.
	ErrBranchNotFound = errors.New("branch not found")

	// ErrWorkspaceNotFound is returned when a workspace/worktree doesn't exist.
	ErrWorkspaceNotFound = errors.New("workspace not found")

	// ErrWorkspaceExists is returned when trying to create an existing workspace.
	ErrWorkspaceExists = errors.New("workspace already exists")

	// ErrMergeConflict is returned when an operation fails due to merge conflicts.
	ErrMergeConflict = errors.New("merge conflicts exist")

	// ErrNothingToCommit is returned when there are no changes to commit.
	ErrNothingToCommit = errors.New("nothing to commit")

	// ErrNotSupported is returned when an operation isn't supported by the VCS.
	ErrNotSupported = errors.New("operation not supported by this VCS")

	// ErrCommandFailed is returned when a VCS command fails.
	ErrCommandFailed = errors.New("vcs command failed")
)

// CommandError wraps an error from a VCS command with additional context.
type CommandError struct {
	VCS     VCSType
	Command string
	Args    []string
	Stderr  string
	Err     error
}

func (e *CommandError) Error() string {
	return e.VCS.String() + " " + e.Command + ": " + e.Err.Error() + ": " + e.Stderr
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

// String returns the string representation of VCSType.
func (v VCSType) String() string {
	return string(v)
}
