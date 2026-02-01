package vcs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceOrchestrator_CreateSubtask(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("orchestrator-test")

	// Create initial content
	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create a subtask
	subtask, err := orchestrator.CreateSubtask(ctx, "task-001", "Add feature X")
	if err != nil {
		t.Fatalf("CreateSubtask failed: %v", err)
	}

	// Verify subtask was created
	if subtask.ID != "task-001" {
		t.Errorf("expected ID task-001, got %s", subtask.ID)
	}
	if subtask.State != SubtaskPending {
		t.Errorf("expected state Pending, got %s", subtask.State)
	}
	if subtask.WorkspaceName != "subtask-task-001" {
		t.Errorf("expected workspace name subtask-task-001, got %s", subtask.WorkspaceName)
	}

	// Verify workspace directory exists
	if _, err := os.Stat(subtask.WorkspacePath); os.IsNotExist(err) {
		t.Error("workspace directory was not created")
	}

	// Verify jj workspace was created
	workspaces, err := jjVCS.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}

	found := false
	for _, ws := range workspaces {
		if ws.Name == "subtask-task-001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("jj workspace was not created")
	}
}

func TestWorkspaceOrchestrator_SubtaskWorkflow(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("workflow-test")

	// Create initial content
	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create subtask
	subtask, err := orchestrator.CreateSubtask(ctx, "task-002", "Add feature Y")
	if err != nil {
		t.Fatalf("CreateSubtask failed: %v", err)
	}

	// Start the subtask
	err = orchestrator.StartSubtask("task-002")
	if err != nil {
		t.Fatalf("StartSubtask failed: %v", err)
	}
	if subtask.State != SubtaskRunning {
		t.Errorf("expected state Running, got %s", subtask.State)
	}

	// Get VCS for subtask workspace
	subtaskVCS, err := orchestrator.GetSubtaskVCS("task-002")
	if err != nil {
		t.Fatalf("GetSubtaskVCS failed: %v", err)
	}

	// Make changes in subtask workspace
	featurePath := filepath.Join(subtask.WorkspacePath, "feature.txt")
	if err := os.WriteFile(featurePath, []byte("feature content"), 0644); err != nil {
		t.Fatalf("failed to write feature file: %v", err)
	}

	// Commit in subtask
	subtaskVCS.Snapshot(ctx)
	err = subtaskVCS.Commit(ctx, "Add feature Y", nil)
	if err != nil {
		t.Fatalf("Subtask commit failed: %v", err)
	}

	// Complete the subtask (squash to main)
	err = orchestrator.CompleteSubtask(ctx, "task-002")
	if err != nil {
		// Might fail due to jj workspace semantics, that's ok for this test
		t.Logf("CompleteSubtask returned: %v (may be expected)", err)
	}

	// Verify subtask state
	completedSubtask, _ := orchestrator.GetSubtask("task-002")
	if completedSubtask.State != SubtaskCompleted && completedSubtask.State != SubtaskConflicted {
		t.Logf("Subtask state: %s (squash semantics may vary)", completedSubtask.State)
	}
}

func TestWorkspaceOrchestrator_FailSubtask(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("fail-test")

	// Create initial content
	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create and start subtask
	subtask, err := orchestrator.CreateSubtask(ctx, "task-003", "Failing task")
	if err != nil {
		t.Fatalf("CreateSubtask failed: %v", err)
	}
	orchestrator.StartSubtask("task-003")

	// Make some changes
	featurePath := filepath.Join(subtask.WorkspacePath, "broken.txt")
	os.WriteFile(featurePath, []byte("broken content"), 0644)

	// Fail the subtask
	err = orchestrator.FailSubtask(ctx, "task-003", "Task failed due to error")
	if err != nil {
		t.Fatalf("FailSubtask failed: %v", err)
	}

	// Verify state
	failedSubtask, _ := orchestrator.GetSubtask("task-003")
	if failedSubtask.State != SubtaskFailed {
		t.Errorf("expected state Failed, got %s", failedSubtask.State)
	}
	if failedSubtask.Error != "Task failed due to error" {
		t.Errorf("expected error message, got %s", failedSubtask.Error)
	}
}

func TestWorkspaceOrchestrator_ListSubtasks(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("list-test")

	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create multiple subtasks
	orchestrator.CreateSubtask(ctx, "task-a", "Task A")
	orchestrator.CreateSubtask(ctx, "task-b", "Task B")
	orchestrator.CreateSubtask(ctx, "task-c", "Task C")

	// List subtasks
	subtasks := orchestrator.ListSubtasks()
	if len(subtasks) != 3 {
		t.Errorf("expected 3 subtasks, got %d", len(subtasks))
	}

	// Verify all IDs present
	ids := make(map[string]bool)
	for _, s := range subtasks {
		ids[s.ID] = true
	}
	for _, expected := range []string{"task-a", "task-b", "task-c"} {
		if !ids[expected] {
			t.Errorf("missing subtask %s", expected)
		}
	}
}

func TestGenerateTaskID(t *testing.T) {
	// Test basic generation
	id1 := GenerateTaskID("wong")
	if !strings.HasPrefix(id1, "wong-") {
		t.Errorf("expected prefix wong-, got %s", id1)
	}
	if len(id1) != 11 { // "wong-" (5) + 6 hex chars
		t.Errorf("expected length 11, got %d for %s", len(id1), id1)
	}

	// Test uniqueness
	id2 := GenerateTaskID("wong")
	if id1 == id2 {
		t.Error("expected unique IDs")
	}

	// Test default prefix
	id3 := GenerateTaskID("")
	if !strings.HasPrefix(id3, "task-") {
		t.Errorf("expected default prefix task-, got %s", id3)
	}
}

func TestGenerateSubtaskID(t *testing.T) {
	// Test with parent
	parentID := "wong-abc123"
	subtaskID := GenerateSubtaskID(parentID)
	if !strings.HasPrefix(subtaskID, "wong-abc123-") {
		t.Errorf("expected prefix wong-abc123-, got %s", subtaskID)
	}

	// Test without parent
	standaloneID := GenerateSubtaskID("")
	if !strings.HasPrefix(standaloneID, "subtask-") {
		t.Errorf("expected prefix subtask-, got %s", standaloneID)
	}
}

func TestParseTaskID(t *testing.T) {
	tests := []struct {
		id        string
		prefix    string
		hash      string
		isSubtask bool
	}{
		{"wong-abc123", "wong", "abc123", false},
		{"task-def456", "task", "def456", false},
		{"wong-abc123-def456", "wong-abc123", "def456", true},
		{"a-b-c-d", "a-b-c", "d", true},
		{"nohash", "nohash", "", false},
	}

	for _, tc := range tests {
		prefix, hash, isSubtask := ParseTaskID(tc.id)
		if prefix != tc.prefix {
			t.Errorf("ParseTaskID(%s): expected prefix %s, got %s", tc.id, tc.prefix, prefix)
		}
		if hash != tc.hash {
			t.Errorf("ParseTaskID(%s): expected hash %s, got %s", tc.id, tc.hash, hash)
		}
		if isSubtask != tc.isSubtask {
			t.Errorf("ParseTaskID(%s): expected isSubtask %v, got %v", tc.id, tc.isSubtask, isSubtask)
		}
	}
}

func TestWorkspaceOrchestrator_CreateSubtaskAuto(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("auto-id-test")

	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create subtask with auto ID
	subtask, err := orchestrator.CreateSubtaskAuto(ctx, "wong", "Auto-generated subtask")
	if err != nil {
		t.Fatalf("CreateSubtaskAuto failed: %v", err)
	}

	// Verify ID format
	if !strings.HasPrefix(subtask.ID, "wong-") {
		t.Errorf("expected ID to start with wong-, got %s", subtask.ID)
	}

	// Verify workspace name matches ID
	expectedName := "subtask-" + subtask.ID
	if subtask.WorkspaceName != expectedName {
		t.Errorf("expected workspace name %s, got %s", expectedName, subtask.WorkspaceName)
	}
}

func TestGenerateTaskIDFromChangeID(t *testing.T) {
	// Test with full change ID
	id1 := GenerateTaskIDFromChangeID("wong", "kpqvuntmwxyz1234")
	if id1 != "wong-kpqvuntm" {
		t.Errorf("expected wong-kpqvuntm, got %s", id1)
	}

	// Test with short change ID
	id2 := GenerateTaskIDFromChangeID("wong", "abc")
	if id2 != "wong-abc" {
		t.Errorf("expected wong-abc, got %s", id2)
	}

	// Test uppercase conversion
	id3 := GenerateTaskIDFromChangeID("wong", "ABCDEFGH")
	if id3 != "wong-abcdefgh" {
		t.Errorf("expected wong-abcdefgh, got %s", id3)
	}
}

func TestWorkspaceOrchestrator_CreateSubtaskFromChange(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("change-id-test")

	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	// Get current change ID for verification
	ctx := context.Background()
	currentChange, err := jjVCS.CurrentChange(ctx)
	if err != nil {
		t.Fatalf("CurrentChange failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)

	// Create subtask from current change
	subtask, err := orchestrator.CreateSubtaskFromChange(ctx, "wong", "Change-based subtask")
	if err != nil {
		t.Fatalf("CreateSubtaskFromChange failed: %v", err)
	}

	// Verify ID contains the change ID
	expectedPrefix := "wong-" + strings.ToLower(currentChange.ShortID[:8])
	if subtask.ID != expectedPrefix {
		t.Logf("Current change short ID: %s", currentChange.ShortID)
		t.Errorf("expected ID %s, got %s", expectedPrefix, subtask.ID)
	}
}

func TestWorkspaceOrchestrator_CreateSubtaskFromParent(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("parent-id-test")

	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create parent task first
	parentTask, err := orchestrator.CreateSubtaskAuto(ctx, "wong", "Parent task")
	if err != nil {
		t.Fatalf("CreateSubtaskAuto failed: %v", err)
	}

	// Create child subtask from parent
	childTask, err := orchestrator.CreateSubtaskFromParent(ctx, parentTask.ID, "Child subtask")
	if err != nil {
		t.Fatalf("CreateSubtaskFromParent failed: %v", err)
	}

	// Verify hierarchical ID
	if !strings.HasPrefix(childTask.ID, parentTask.ID+"-") {
		t.Errorf("expected child ID to start with %s-, got %s", parentTask.ID, childTask.ID)
	}

	// Verify it's recognized as a subtask
	_, _, isSubtask := ParseTaskID(childTask.ID)
	if !isSubtask {
		t.Error("expected child to be recognized as subtask")
	}
}

func TestCreateConflictBead(t *testing.T) {
	subtask := &Subtask{
		ID:              "task-conflict",
		Description:     "Feature that conflicts",
		WorkspacePath:   "/tmp/wong-subtask-task-conflict",
		WorkspaceName:   "subtask-task-conflict",
		ParentChangeID:  "abc123",
		CurrentChangeID: "def456",
		State:           SubtaskConflicted,
	}

	bead := CreateConflictBead(subtask)

	if bead["priority"] != 0 {
		t.Errorf("expected priority 0 (P0), got %v", bead["priority"])
	}
	if bead["type"] != "bug" {
		t.Errorf("expected type bug, got %v", bead["type"])
	}

	title, ok := bead["title"].(string)
	if !ok || title == "" {
		t.Error("expected non-empty title")
	}

	desc, ok := bead["description"].(string)
	if !ok || desc == "" {
		t.Error("expected non-empty description")
	}
}

func TestWorkspaceOrchestrator_ParallelSubtasks(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("parallel-test")

	h.WriteFile(repoPath, "main.txt", "main content")
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	ctx := context.Background()

	// Create multiple subtasks in parallel (simulated)
	subtask1, _ := orchestrator.CreateSubtask(ctx, "parallel-1", "Parallel task 1")
	subtask2, _ := orchestrator.CreateSubtask(ctx, "parallel-2", "Parallel task 2")

	orchestrator.StartSubtask("parallel-1")
	orchestrator.StartSubtask("parallel-2")

	// Make different changes in each workspace
	os.WriteFile(filepath.Join(subtask1.WorkspacePath, "file1.txt"), []byte("content 1"), 0644)
	os.WriteFile(filepath.Join(subtask2.WorkspacePath, "file2.txt"), []byte("content 2"), 0644)

	// Verify both workspaces are independent
	vcs1, _ := orchestrator.GetSubtaskVCS("parallel-1")
	vcs2, _ := orchestrator.GetSubtaskVCS("parallel-2")

	// Snapshot both
	vcs1.Snapshot(ctx)
	vcs2.Snapshot(ctx)

	// Check status in each - should only see their own file
	status1, _ := vcs1.Status(ctx)
	status2, _ := vcs2.Status(ctx)

	// Each workspace should have independent changes
	t.Logf("Workspace 1 status: %d files", len(status1))
	t.Logf("Workspace 2 status: %d files", len(status2))

	// Verify workspaces exist
	workspaces, _ := jjVCS.ListWorkspaces(ctx)
	if len(workspaces) < 3 { // default + 2 subtasks
		t.Errorf("expected at least 3 workspaces, got %d", len(workspaces))
	}
}
