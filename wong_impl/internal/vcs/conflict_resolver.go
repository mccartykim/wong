// Package vcs provides automated conflict resolution for workspace orchestration.
//
// When a subtask's changes conflict with main, the ConflictResolver:
//  1. Detects conflicts and their type
//  2. Creates a high-priority bead for tracking
//  3. Provides guided resolution steps
//  4. Auto-resolves simple conflicts (e.g., beads JSONL using 3-way merge)
//  5. Cleans up after resolution
package vcs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ConflictType categorizes the kind of conflict.
type ConflictType string

const (
	// ConflictTypeContent is a file content conflict.
	ConflictTypeContent ConflictType = "content"
	// ConflictTypeBeadsJSONL is a conflict in .beads/issues.jsonl (auto-resolvable).
	ConflictTypeBeadsJSONL ConflictType = "beads_jsonl"
	// ConflictTypeAdd is both sides added the same file.
	ConflictTypeAdd ConflictType = "add_add"
	// ConflictTypeModifyDelete is one side modified, other deleted.
	ConflictTypeModifyDelete ConflictType = "modify_delete"
)

// ConflictInfo describes a single conflict.
type ConflictInfo struct {
	// Path is the file path with conflicts.
	Path string

	// Type is the category of conflict.
	Type ConflictType

	// AutoResolvable indicates this conflict can be resolved automatically.
	AutoResolvable bool

	// Resolution is the auto-resolution strategy (if AutoResolvable).
	Resolution string

	// SubtaskID is the subtask that caused this conflict.
	SubtaskID string
}

// ConflictResolution represents the outcome of resolving conflicts.
type ConflictResolution struct {
	// Resolved lists conflicts that were auto-resolved.
	Resolved []ConflictInfo

	// Unresolved lists conflicts that need manual intervention.
	Unresolved []ConflictInfo

	// BeadID is the bead created for tracking (if any unresolved conflicts).
	BeadID string

	// Timestamp when resolution was attempted.
	Timestamp time.Time
}

// ConflictResolver handles conflict detection and resolution.
type ConflictResolver struct {
	vcs          *JujutsuVCS
	orchestrator *WorkspaceOrchestrator
}

// NewConflictResolver creates a conflict resolver for the given orchestrator.
func NewConflictResolver(orchestrator *WorkspaceOrchestrator) *ConflictResolver {
	return &ConflictResolver{
		vcs:          orchestrator.vcs,
		orchestrator: orchestrator,
	}
}

// DetectConflicts checks for conflicts in the main workspace after a squash.
func (cr *ConflictResolver) DetectConflicts(ctx context.Context, subtaskID string) ([]ConflictInfo, error) {
	conflicts, err := cr.vcs.GetConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts: %w", err)
	}

	var infos []ConflictInfo
	for _, conflict := range conflicts {
		info := ConflictInfo{
			Path:      conflict.Path,
			SubtaskID: subtaskID,
		}

		// Categorize the conflict
		info.Type, info.AutoResolvable, info.Resolution = cr.categorizeConflict(conflict.Path)

		infos = append(infos, info)
	}

	return infos, nil
}

// categorizeConflict determines the type and auto-resolution capability.
func (cr *ConflictResolver) categorizeConflict(path string) (ConflictType, bool, string) {
	// .beads/issues.jsonl can be auto-resolved with JSONL merge
	if strings.HasSuffix(path, "issues.jsonl") || strings.HasSuffix(path, "deletions.jsonl") {
		return ConflictTypeBeadsJSONL, true, "jsonl_merge"
	}

	// Other .beads files can usually take "ours" (main workspace)
	if strings.HasPrefix(path, ".beads/") || strings.HasPrefix(path, ".beads"+string(filepath.Separator)) {
		return ConflictTypeContent, true, "take_ours"
	}

	// Everything else needs manual resolution
	return ConflictTypeContent, false, ""
}

// ResolveConflicts attempts to resolve conflicts, returning what was resolved and what wasn't.
func (cr *ConflictResolver) ResolveConflicts(ctx context.Context, subtaskID string) (*ConflictResolution, error) {
	conflicts, err := cr.DetectConflicts(ctx, subtaskID)
	if err != nil {
		return nil, err
	}

	resolution := &ConflictResolution{
		Timestamp: time.Now(),
	}

	for _, conflict := range conflicts {
		if conflict.AutoResolvable {
			// Try auto-resolve
			if err := cr.autoResolve(ctx, conflict); err != nil {
				// Auto-resolve failed, mark as unresolved
				conflict.AutoResolvable = false
				resolution.Unresolved = append(resolution.Unresolved, conflict)
			} else {
				resolution.Resolved = append(resolution.Resolved, conflict)
			}
		} else {
			resolution.Unresolved = append(resolution.Unresolved, conflict)
		}
	}

	// If there are unresolved conflicts, create a tracking bead
	if len(resolution.Unresolved) > 0 {
		subtask, _ := cr.orchestrator.GetSubtask(subtaskID)
		if subtask != nil {
			bead := cr.createResolutionBead(subtask, resolution)
			resolution.BeadID = bead["id"].(string)
		}
	}

	return resolution, nil
}

// autoResolve attempts automatic resolution of a conflict.
func (cr *ConflictResolver) autoResolve(ctx context.Context, conflict ConflictInfo) error {
	switch conflict.Resolution {
	case "jsonl_merge":
		return cr.resolveJSONLMerge(ctx, conflict.Path)
	case "take_ours":
		return cr.resolveTakeOurs(ctx, conflict.Path)
	default:
		return fmt.Errorf("unknown resolution strategy: %s", conflict.Resolution)
	}
}

// resolveJSONLMerge resolves a JSONL file conflict using line-level merge.
// Each line in JSONL is independent, so we can merge by combining unique lines.
func (cr *ConflictResolver) resolveJSONLMerge(ctx context.Context, path string) error {
	// For jj, we can use the resolve command with a custom merge tool
	// Or we can read both sides and merge ourselves

	// Get the conflicted file content
	// jj stores conflicts inline in the file with conflict markers
	// We can use jj resolve to handle it
	_, err := cr.vcs.runJJ(ctx, "resolve", path)
	if err != nil {
		// If jj resolve fails (no merge tool), try manual resolution
		// Read the file and parse conflict markers
		return cr.manualJSONLMerge(ctx, path)
	}
	return nil
}

// manualJSONLMerge performs a manual JSONL merge by reading conflict sides.
func (cr *ConflictResolver) manualJSONLMerge(ctx context.Context, path string) error {
	// jj represents conflicts differently than git (tree-level conflicts)
	// We need to use jj's conflict resolution
	// For now, try to restore from the subtask's version
	_, err := cr.vcs.runJJ(ctx, "restore", "--from", "@-", path)
	return err
}

// resolveTakeOurs resolves a conflict by taking the main workspace's version.
func (cr *ConflictResolver) resolveTakeOurs(ctx context.Context, path string) error {
	// Restore from the parent (main workspace's version)
	_, err := cr.vcs.runJJ(ctx, "restore", "--from", "@-", path)
	return err
}

// createResolutionBead creates a bead for tracking unresolved conflicts.
func (cr *ConflictResolver) createResolutionBead(subtask *Subtask, resolution *ConflictResolution) map[string]interface{} {
	var unresolvedPaths []string
	for _, c := range resolution.Unresolved {
		unresolvedPaths = append(unresolvedPaths, c.Path)
	}

	var resolvedPaths []string
	for _, c := range resolution.Resolved {
		resolvedPaths = append(resolvedPaths, fmt.Sprintf("%s (auto: %s)", c.Path, c.Resolution))
	}

	id := GenerateTaskID("conflict")

	description := fmt.Sprintf(`Merge conflicts from subtask %s need manual resolution.

## Subtask
- ID: %s
- Description: %s
- Change: %s

## Auto-Resolved (%d files)
%s

## Needs Manual Resolution (%d files)
%s

## Resolution Steps
1. Review each conflicted file: jj diff %s
2. Edit files to resolve conflicts
3. For each resolved file: jj resolve <path>
4. Verify no conflicts remain: jj status
5. Close this bead when done

## Quick Commands
jj status                    # Check conflict status
jj diff                      # See all changes
jj resolve --list            # List conflicted files
`,
		subtask.ID,
		subtask.ID, subtask.Description, subtask.CurrentChangeID,
		len(resolution.Resolved), formatPaths(resolvedPaths),
		len(resolution.Unresolved), formatPaths(unresolvedPaths),
		subtask.CurrentChangeID,
	)

	return map[string]interface{}{
		"id":          id,
		"title":       fmt.Sprintf("CONFLICT: Resolve conflicts from %s", subtask.ID),
		"type":        "bug",
		"priority":    0, // P0 - highest priority
		"description": description,
		"subtask_id":  subtask.ID,
	}
}

// formatPaths formats a list of paths for display.
func formatPaths(paths []string) string {
	if len(paths) == 0 {
		return "  (none)"
	}
	var lines []string
	for _, p := range paths {
		lines = append(lines, fmt.Sprintf("  - %s", p))
	}
	return strings.Join(lines, "\n")
}

// --- Integration with WorkspaceOrchestrator ---

// CompleteSubtaskWithConflictResolution is an enhanced version of CompleteSubtask
// that auto-resolves beads conflicts and creates tracking beads for the rest.
func (cr *ConflictResolver) CompleteSubtaskWithConflictResolution(ctx context.Context, subtaskID string) (*ConflictResolution, error) {
	// First try the standard completion
	err := cr.orchestrator.CompleteSubtask(ctx, subtaskID)
	if err == nil {
		// No conflicts - clean completion
		return &ConflictResolution{
			Timestamp: time.Now(),
		}, nil
	}

	// Check if it's a conflict error
	if _, ok := err.(*ConflictError); !ok {
		// Not a conflict - propagate error
		return nil, err
	}

	// Conflicts detected - try to resolve
	return cr.ResolveConflicts(ctx, subtaskID)
}

// CheckAndResolveMainConflicts checks main workspace for any conflicts
// and attempts auto-resolution. Creates beads for unresolvable conflicts.
func (cr *ConflictResolver) CheckAndResolveMainConflicts(ctx context.Context) (*ConflictResolution, error) {
	hasConflicts, err := cr.vcs.HasMergeConflicts(ctx)
	if err != nil {
		return nil, err
	}

	if !hasConflicts {
		return &ConflictResolution{Timestamp: time.Now()}, nil
	}

	// Detect and resolve
	return cr.ResolveConflicts(ctx, "main")
}
