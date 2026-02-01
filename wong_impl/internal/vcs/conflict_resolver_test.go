package vcs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCategorizeConflict(t *testing.T) {
	cr := &ConflictResolver{}

	tests := []struct {
		path           string
		expectedType   ConflictType
		expectedAuto   bool
		expectedStrat  string
	}{
		{".beads/issues.jsonl", ConflictTypeBeadsJSONL, true, "jsonl_merge"},
		{".beads/deletions.jsonl", ConflictTypeBeadsJSONL, true, "jsonl_merge"},
		{".beads/metadata.json", ConflictTypeContent, true, "take_ours"},
		{".beads/config.yaml", ConflictTypeContent, true, "take_ours"},
		{"src/main.go", ConflictTypeContent, false, ""},
		{"README.md", ConflictTypeContent, false, ""},
	}

	for _, tc := range tests {
		cType, autoResolvable, strategy := cr.categorizeConflict(tc.path)
		if cType != tc.expectedType {
			t.Errorf("categorizeConflict(%s): expected type %s, got %s", tc.path, tc.expectedType, cType)
		}
		if autoResolvable != tc.expectedAuto {
			t.Errorf("categorizeConflict(%s): expected autoResolvable %v, got %v", tc.path, tc.expectedAuto, autoResolvable)
		}
		if strategy != tc.expectedStrat {
			t.Errorf("categorizeConflict(%s): expected strategy %s, got %s", tc.path, tc.expectedStrat, strategy)
		}
	}
}

func TestCreateResolutionBead(t *testing.T) {
	cr := &ConflictResolver{}

	subtask := &Subtask{
		ID:              "wong-abc123",
		Description:     "Add feature X",
		WorkspacePath:   "/tmp/wong-subtask-abc123",
		WorkspaceName:   "subtask-abc123",
		ParentChangeID:  "parent123",
		CurrentChangeID: "change456",
		State:           SubtaskConflicted,
	}

	resolution := &ConflictResolution{
		Resolved: []ConflictInfo{
			{Path: ".beads/issues.jsonl", Type: ConflictTypeBeadsJSONL, Resolution: "jsonl_merge"},
		},
		Unresolved: []ConflictInfo{
			{Path: "src/main.go", Type: ConflictTypeContent},
			{Path: "src/lib.go", Type: ConflictTypeContent},
		},
	}

	bead := cr.createResolutionBead(subtask, resolution)

	// Verify bead fields
	if bead["priority"] != 0 {
		t.Errorf("expected P0 priority, got %v", bead["priority"])
	}
	if bead["type"] != "bug" {
		t.Errorf("expected type bug, got %v", bead["type"])
	}

	title := bead["title"].(string)
	if title == "" {
		t.Error("expected non-empty title")
	}

	desc := bead["description"].(string)
	if desc == "" {
		t.Error("expected non-empty description")
	}

	// Verify description contains key info
	if !containsAll(desc, []string{"wong-abc123", "Add feature X", "change456", "src/main.go", "src/lib.go", "issues.jsonl"}) {
		t.Error("description missing key information")
	}

	if bead["subtask_id"] != "wong-abc123" {
		t.Errorf("expected subtask_id wong-abc123, got %v", bead["subtask_id"])
	}
}

func TestFormatPaths(t *testing.T) {
	// Empty paths
	result := formatPaths(nil)
	if result != "  (none)" {
		t.Errorf("expected '  (none)', got '%s'", result)
	}

	// Multiple paths
	result = formatPaths([]string{"file1.go", "file2.go"})
	if result != "  - file1.go\n  - file2.go" {
		t.Errorf("unexpected format: %s", result)
	}
}

func TestConflictResolver_Integration(t *testing.T) {
	if _, err := os.Stat("/root/.cargo/bin/jj"); err != nil {
		t.Skip("jj not installed")
	}

	h := NewTestHelper(t)
	repoPath := h.CreateJJRepo("conflict-test")

	// Create initial content
	h.WriteFile(repoPath, "main.txt", "main content")
	if err := os.MkdirAll(filepath.Join(repoPath, ".beads"), 0755); err != nil {
		t.Fatalf("failed to create .beads dir: %v", err)
	}
	h.WriteFile(repoPath, ".beads/issues.jsonl", `{"id":"issue-1"}`)
	h.runCmd(repoPath, "jj", "commit", "-m", "Initial commit")

	jjVCS, err := NewJujutsuVCS(repoPath)
	if err != nil {
		t.Fatalf("NewJujutsuVCS failed: %v", err)
	}

	orchestrator := NewWorkspaceOrchestrator(jjVCS, h.tempDir)
	resolver := NewConflictResolver(orchestrator)

	// Verify resolver was created
	if resolver.vcs == nil {
		t.Error("resolver.vcs should not be nil")
	}
	if resolver.orchestrator == nil {
		t.Error("resolver.orchestrator should not be nil")
	}
}

// containsAll checks if s contains all substrings.
func containsAll(s string, substrings []string) bool {
	for _, sub := range substrings {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
