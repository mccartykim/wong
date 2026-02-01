package wongdb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Decorator wraps jj commands with optional pre/post wong-db sync.
type Decorator struct {
	db    *WongDB
	jjBin string
}

// jjWriteCommands are jj subcommands that modify the repository.
var jjWriteCommands = []string{
	"new", "commit", "describe", "squash", "rebase",
	"edit", "abandon", "restore", "split", "absorb",
	"resolve", "backout", "bookmark", "branch", "git",
}

// jjReadCommands are jj subcommands that do not modify the repository.
var jjReadCommands = []string{
	"log", "show", "diff", "status", "file",
	"config", "op", "workspace",
}

// NewDecorator creates a new Decorator that wraps jj with wong-db sync.
func NewDecorator(db *WongDB) *Decorator {
	jjBin := "jj"
	if db != nil && db.jjBin != "" {
		jjBin = db.jjBin
	}
	return &Decorator{
		db:    db,
		jjBin: jjBin,
	}
}

// Run is the main entry point for the decorator. It executes the jj command
// with the given args, passing through stdin/stdout/stderr. If the subcommand
// is a write command and jj exits successfully, it syncs .wong/ to wong-db.
func (d *Decorator) Run(ctx context.Context, args []string) error {
	subcmd := d.extractSubcommand(args)

	cmd := exec.CommandContext(ctx, d.jjBin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	// If jj succeeded and this was a write command, sync wong-db.
	if err == nil && d.isWriteCommand(subcmd) {
		if syncErr := d.db.Sync(ctx); syncErr != nil {
			fmt.Fprintf(os.Stderr, "wong: post-sync warning: %v\n", syncErr)
		}
	}

	// Preserve jj's exit code: if jj failed, return the exec error
	// (which includes the exit code). Sync errors do not change the exit code.
	return err
}

// isWriteCommand reports whether subcmd is a jj command that modifies the repo.
func (d *Decorator) isWriteCommand(subcmd string) bool {
	for _, w := range jjWriteCommands {
		if w == subcmd {
			return true
		}
	}
	return false
}

// extractSubcommand returns the first non-flag argument from args,
// which is the jj subcommand (e.g. "log", "new", "commit").
func (d *Decorator) extractSubcommand(args []string) string {
	for _, arg := range args {
		if len(arg) == 0 {
			continue
		}
		if arg[0] == '-' {
			continue
		}
		return arg
	}
	return ""
}
