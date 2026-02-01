// Package vcs provides workspace orchestration for parallel subtask execution.
//
// # Workspace-Per-Subtask Model
//
// This model uses jj workspaces to isolate subtask execution:
//
//	Orchestrator (main workspace @ repo root)
//	    │
//	    ├── Creates subtask workspace branched from current @
//	    │
//	    ├── Subtask executes in /tmp/<task-id>/
//	    │   - Has its own working copy
//	    │   - Can make multiple commits
//	    │   - Isolated from other subtasks
//	    │
//	    └── On completion:
//	        - Success: squash changes to main, cleanup
//	        - Failure: abandon changes, cleanup, create conflict bead
//
// # Benefits
//
//   - Clean isolation: subtasks can't interfere with each other
//   - Parallel execution: multiple workspaces can run simultaneously
//   - Atomic merges: squash ensures clean history
//   - Easy rollback: just abandon the workspace on failure
//   - Native jj: uses jj's built-in workspace model
//
// # Conflict Handling
//
// When squashing causes conflicts:
//  1. Create high-priority bead for conflict resolution
//  2. Keep subtask workspace alive for reference
//  3. Guide user through resolution in main workspace
//  4. After resolution, cleanup subtask workspace
package vcs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateTaskID generates a unique task ID with a hash suffix.
// Format: prefix-xxxx where xxxx is a random hex string.
// This matches beads' ID format (e.g., wong-abc123).
func GenerateTaskID(prefix string) string {
	bytes := make([]byte, 3) // 6 hex chars
	rand.Read(bytes)
	hash := hex.EncodeToString(bytes)
	if prefix == "" {
		prefix = "task"
	}
	return fmt.Sprintf("%s-%s", prefix, hash)
}

// GenerateTaskIDFromChangeID creates a task ID using a jj change ID.
// This creates a direct link between the task and the jj change it branched from.
// Format: prefix-<short-change-id> (e.g., wong-kpqvuntm)
func GenerateTaskIDFromChangeID(prefix, changeID string) string {
	if prefix == "" {
		prefix = "task"
	}
	// Use first 8 chars of change ID (jj short format)
	shortID := changeID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	// Lowercase to match jj convention
	shortID = strings.ToLower(shortID)
	return fmt.Sprintf("%s-%s", prefix, shortID)
}

// GenerateSubtaskID generates a unique subtask ID.
// If parentTaskID is provided, creates a hierarchical ID.
// Example: wong-abc123 -> wong-abc123-def456
func GenerateSubtaskID(parentTaskID string) string {
	bytes := make([]byte, 3)
	rand.Read(bytes)
	hash := hex.EncodeToString(bytes)

	if parentTaskID != "" {
		return fmt.Sprintf("%s-%s", parentTaskID, hash)
	}
	return fmt.Sprintf("subtask-%s", hash)
}

// GenerateSubtaskIDFromChangeID creates a hierarchical subtask ID using a jj change ID.
// Example: wong-kpqvuntm -> wong-kpqvuntm-xyzwmnop (where xyzwmnop is the subtask's change ID)
func GenerateSubtaskIDFromChangeID(parentTaskID, changeID string) string {
	shortID := changeID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	shortID = strings.ToLower(shortID)

	if parentTaskID != "" {
		return fmt.Sprintf("%s-%s", parentTaskID, shortID)
	}
	return fmt.Sprintf("subtask-%s", shortID)
}

// ParseTaskID extracts components from a task ID.
// Returns (prefix, hash, isSubtask).
func ParseTaskID(id string) (prefix string, hash string, isSubtask bool) {
	parts := strings.Split(id, "-")
	if len(parts) < 2 {
		return id, "", false
	}
	if len(parts) == 2 {
		return parts[0], parts[1], false
	}
	// Hierarchical ID: prefix-hash-subhash
	return strings.Join(parts[:len(parts)-1], "-"), parts[len(parts)-1], true
}

// SubtaskState represents the state of a subtask workspace.
type SubtaskState string

const (
	SubtaskPending    SubtaskState = "pending"
	SubtaskRunning    SubtaskState = "running"
	SubtaskCompleted  SubtaskState = "completed"
	SubtaskFailed     SubtaskState = "failed"
	SubtaskConflicted SubtaskState = "conflicted"
)

// Subtask represents a unit of work in an isolated workspace.
type Subtask struct {
	// ID is the unique identifier for this subtask (used for workspace name).
	ID string

	// Description of what this subtask does.
	Description string

	// WorkspacePath is the filesystem path where the workspace lives.
	WorkspacePath string

	// WorkspaceName is the jj workspace name.
	WorkspaceName string

	// ParentChangeID is the change ID this subtask branched from.
	ParentChangeID string

	// CurrentChangeID is the current change ID in the subtask workspace.
	CurrentChangeID string

	// State is the current state of the subtask.
	State SubtaskState

	// CreatedAt is when the subtask was created.
	CreatedAt time.Time

	// CompletedAt is when the subtask finished (success or failure).
	CompletedAt time.Time

	// Error contains any error message if the subtask failed.
	Error string
}

// WorkspaceOrchestrator manages subtask workspaces.
type WorkspaceOrchestrator struct {
	// vcs is the VCS instance for the main workspace.
	vcs *JujutsuVCS

	// basePath is where subtask workspaces are created (e.g., /tmp).
	basePath string

	// subtasks tracks all active subtasks.
	subtasks map[string]*Subtask
}

// NewWorkspaceOrchestrator creates a new orchestrator for the given jj repo.
func NewWorkspaceOrchestrator(vcs *JujutsuVCS, basePath string) *WorkspaceOrchestrator {
	if basePath == "" {
		basePath = "/tmp"
	}
	return &WorkspaceOrchestrator{
		vcs:      vcs,
		basePath: basePath,
		subtasks: make(map[string]*Subtask),
	}
}

// CreateSubtaskAuto creates a new subtask with an auto-generated ID.
// The ID format matches beads: prefix-hash (e.g., wong-abc123).
func (wo *WorkspaceOrchestrator) CreateSubtaskAuto(ctx context.Context, prefix, description string) (*Subtask, error) {
	id := GenerateTaskID(prefix)
	return wo.CreateSubtask(ctx, id, description)
}

// CreateSubtaskFromChange creates a subtask with an ID derived from the jj change ID.
// This links the task directly to the jj change it branches from.
// Example: If orchestrator is at change kpqvuntm, creates wong-kpqvuntm
func (wo *WorkspaceOrchestrator) CreateSubtaskFromChange(ctx context.Context, prefix, description string) (*Subtask, error) {
	// Get current change ID from main workspace
	currentChange, err := wo.vcs.CurrentChange(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current change: %w", err)
	}

	id := GenerateTaskIDFromChangeID(prefix, currentChange.ShortID)
	return wo.CreateSubtask(ctx, id, description)
}

// CreateSubtaskFromParent creates a subtask with a hierarchical ID.
// Example: parent wong-abc123 -> subtask wong-abc123-def456
func (wo *WorkspaceOrchestrator) CreateSubtaskFromParent(ctx context.Context, parentID, description string) (*Subtask, error) {
	id := GenerateSubtaskID(parentID)
	return wo.CreateSubtask(ctx, id, description)
}

// CreateSubtaskFromParentChange creates a hierarchical subtask using jj change IDs.
// The subtask ID includes both the parent's change ID and its own new change ID.
func (wo *WorkspaceOrchestrator) CreateSubtaskFromParentChange(ctx context.Context, parentID, description string) (*Subtask, error) {
	// Create the subtask first to get its change ID
	subtask, err := wo.CreateSubtask(ctx, "temp", description)
	if err != nil {
		return nil, err
	}

	// Get the new workspace's change ID
	subtaskVCS, err := wo.GetSubtaskVCS(subtask.ID)
	if err != nil {
		wo.cleanupSubtask(ctx, subtask)
		return nil, err
	}

	newChange, err := subtaskVCS.CurrentChange(ctx)
	if err != nil {
		wo.cleanupSubtask(ctx, subtask)
		return nil, err
	}

	// Generate hierarchical ID using change IDs
	newID := GenerateSubtaskIDFromChangeID(parentID, newChange.ShortID)

	// Update subtask with new ID (rename workspace would be complex, so we track by new ID)
	delete(wo.subtasks, subtask.ID)
	subtask.ID = newID
	wo.subtasks[newID] = subtask

	return subtask, nil
}

// CreateSubtask creates a new isolated workspace for a subtask.
//
// The workflow:
//  1. Get current change ID from main workspace
//  2. Create workspace directory in basePath
//  3. Create jj workspace pointing to that directory
//  4. The new workspace starts at the same change as main
//
// The ID should be unique and collision-resistant. Use CreateSubtaskAuto
// or CreateSubtaskFromParent for auto-generated IDs.
//
// Returns the Subtask with all metadata populated.
func (wo *WorkspaceOrchestrator) CreateSubtask(ctx context.Context, id, description string) (*Subtask, error) {
	// Get current change ID to branch from
	currentChange, err := wo.vcs.CurrentChange(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current change: %w", err)
	}

	// Create workspace directory
	workspacePath := filepath.Join(wo.basePath, "wong-subtask-"+id)
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Create jj workspace
	workspaceName := "subtask-" + id
	if err := wo.vcs.CreateWorkspace(ctx, workspaceName, workspacePath); err != nil {
		os.RemoveAll(workspacePath)
		return nil, fmt.Errorf("failed to create jj workspace: %w", err)
	}

	subtask := &Subtask{
		ID:              id,
		Description:     description,
		WorkspacePath:   workspacePath,
		WorkspaceName:   workspaceName,
		ParentChangeID:  currentChange.ID,
		CurrentChangeID: currentChange.ID, // Starts at same change
		State:           SubtaskPending,
		CreatedAt:       time.Now(),
	}

	wo.subtasks[id] = subtask
	return subtask, nil
}

// StartSubtask marks a subtask as running.
func (wo *WorkspaceOrchestrator) StartSubtask(id string) error {
	subtask, ok := wo.subtasks[id]
	if !ok {
		return fmt.Errorf("subtask %s not found", id)
	}
	subtask.State = SubtaskRunning
	return nil
}

// GetSubtaskVCS returns a JujutsuVCS instance for operating within a subtask workspace.
func (wo *WorkspaceOrchestrator) GetSubtaskVCS(id string) (*JujutsuVCS, error) {
	subtask, ok := wo.subtasks[id]
	if !ok {
		return nil, fmt.Errorf("subtask %s not found", id)
	}

	return NewJujutsuVCS(subtask.WorkspacePath)
}

// CompleteSubtask handles successful subtask completion.
//
// The workflow:
//  1. Squash subtask changes into parent change
//  2. If conflicts: mark as conflicted, create resolution bead
//  3. If success: forget workspace, cleanup directory
func (wo *WorkspaceOrchestrator) CompleteSubtask(ctx context.Context, id string) error {
	subtask, ok := wo.subtasks[id]
	if !ok {
		return fmt.Errorf("subtask %s not found", id)
	}

	// Try to squash the subtask's changes into main
	// This is done from the main workspace, targeting the subtask's changes
	err := wo.squashSubtaskToMain(ctx, subtask)
	if err != nil {
		// Check if it's a conflict
		hasConflicts, _ := wo.vcs.HasMergeConflicts(ctx)
		if hasConflicts {
			subtask.State = SubtaskConflicted
			subtask.Error = "Conflicts when squashing to main"
			subtask.CompletedAt = time.Now()
			// Don't cleanup - keep workspace for reference during resolution
			return &ConflictError{
				SubtaskID:   id,
				Description: subtask.Description,
				Message:     "Subtask completed but conflicts occurred when merging to main",
			}
		}
		subtask.State = SubtaskFailed
		subtask.Error = err.Error()
		subtask.CompletedAt = time.Now()
		return err
	}

	// Success - cleanup
	subtask.State = SubtaskCompleted
	subtask.CompletedAt = time.Now()

	return wo.cleanupSubtask(ctx, subtask)
}

// FailSubtask handles subtask failure.
func (wo *WorkspaceOrchestrator) FailSubtask(ctx context.Context, id string, reason string) error {
	subtask, ok := wo.subtasks[id]
	if !ok {
		return fmt.Errorf("subtask %s not found", id)
	}

	subtask.State = SubtaskFailed
	subtask.Error = reason
	subtask.CompletedAt = time.Now()

	// Abandon changes and cleanup
	subtaskVCS, err := wo.GetSubtaskVCS(id)
	if err == nil {
		// Abandon all changes in subtask workspace
		subtaskVCS.Abandon(ctx, "@")
	}

	return wo.cleanupSubtask(ctx, subtask)
}

// squashSubtaskToMain squashes subtask changes into the main workspace.
func (wo *WorkspaceOrchestrator) squashSubtaskToMain(ctx context.Context, subtask *Subtask) error {
	// Get the subtask's current change ID
	subtaskVCS, err := wo.GetSubtaskVCS(subtask.ID)
	if err != nil {
		return err
	}

	currentChange, err := subtaskVCS.CurrentChange(ctx)
	if err != nil {
		return err
	}

	// Update subtask's current change ID
	subtask.CurrentChangeID = currentChange.ID

	// From main workspace, squash the subtask's changes
	// jj squash --from <subtask-change> --into <main-change>
	_, err = wo.vcs.runJJ(ctx, "squash", "--from", currentChange.ID, "--into", subtask.ParentChangeID)
	return err
}

// cleanupSubtask removes the workspace and directory.
func (wo *WorkspaceOrchestrator) cleanupSubtask(ctx context.Context, subtask *Subtask) error {
	// Forget the workspace
	wo.vcs.RemoveWorkspace(ctx, subtask.WorkspaceName)

	// Remove the directory
	os.RemoveAll(subtask.WorkspacePath)

	// Remove from tracking (but keep for history if needed)
	// delete(wo.subtasks, subtask.ID)

	return nil
}

// ResolveConflict is called after user resolves conflicts in main workspace.
func (wo *WorkspaceOrchestrator) ResolveConflict(ctx context.Context, id string) error {
	subtask, ok := wo.subtasks[id]
	if !ok {
		return fmt.Errorf("subtask %s not found", id)
	}

	if subtask.State != SubtaskConflicted {
		return fmt.Errorf("subtask %s is not in conflicted state", id)
	}

	// Verify conflicts are resolved in main
	hasConflicts, err := wo.vcs.HasMergeConflicts(ctx)
	if err != nil {
		return err
	}
	if hasConflicts {
		return fmt.Errorf("conflicts still exist in main workspace")
	}

	// Mark as completed and cleanup
	subtask.State = SubtaskCompleted
	subtask.CompletedAt = time.Now()

	return wo.cleanupSubtask(ctx, subtask)
}

// ListSubtasks returns all tracked subtasks.
func (wo *WorkspaceOrchestrator) ListSubtasks() []*Subtask {
	result := make([]*Subtask, 0, len(wo.subtasks))
	for _, s := range wo.subtasks {
		result = append(result, s)
	}
	return result
}

// GetSubtask returns a specific subtask by ID.
func (wo *WorkspaceOrchestrator) GetSubtask(id string) (*Subtask, bool) {
	s, ok := wo.subtasks[id]
	return s, ok
}

// RefreshMainWorkspace updates the main workspace after external changes.
// Call this if the main workspace might be stale.
func (wo *WorkspaceOrchestrator) RefreshMainWorkspace(ctx context.Context) error {
	// Snapshot to capture any working copy changes
	return wo.vcs.Snapshot(ctx)
}

// ConflictError represents a conflict that occurred during subtask completion.
type ConflictError struct {
	SubtaskID   string
	Description string
	Message     string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict in subtask %s (%s): %s", e.SubtaskID, e.Description, e.Message)
}

// --- Helper functions for beads integration ---

// CreateConflictBead creates a high-priority bead for conflict resolution.
// This would integrate with beads to create an issue.
func CreateConflictBead(subtask *Subtask) map[string]interface{} {
	return map[string]interface{}{
		"title":       fmt.Sprintf("CONFLICT: Resolve merge conflict from subtask %s", subtask.ID),
		"type":        "bug",
		"priority":    0, // P0 - highest priority
		"description": fmt.Sprintf(`Subtask "%s" completed but caused conflicts when merging to main.

## Subtask Details
- ID: %s
- Description: %s
- Workspace: %s (kept for reference)
- Parent Change: %s
- Subtask Change: %s

## Resolution Steps
1. Review conflicts in main workspace: jj status
2. Resolve each conflicted file
3. Run: jj resolve <file> for each resolved file
4. After all resolved, the orchestrator will cleanup the subtask workspace

## Commands
- View subtask changes: jj log -r %s
- Compare with main: jj diff --from %s --to %s
`, subtask.Description, subtask.ID, subtask.Description, subtask.WorkspacePath,
			subtask.ParentChangeID, subtask.CurrentChangeID,
			subtask.CurrentChangeID, subtask.ParentChangeID, subtask.CurrentChangeID),
	}
}
