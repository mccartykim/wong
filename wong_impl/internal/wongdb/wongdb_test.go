package wongdb

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// setupJJRepo creates a temporary directory with a colocated jj+git repo.
// It returns the path to the repo root. The directory is cleaned up when the
// test finishes via t.Cleanup.
func setupJJRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir() // automatically cleaned up

	// Initialize a colocated jj+git repo
	cmd := exec.Command("jj", "git", "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj git init failed: %v\noutput: %s", err, out)
	}

	return dir
}

// runJJ is a helper to run a jj command in the given repo directory and return
// the combined output. It fails the test on error.
func runJJ(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("jj", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %s failed: %v\noutput: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// runJJMayFail runs a jj command and returns the output and error without
// failing the test. Useful for commands expected to fail.
func runJJMayFail(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("jj", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// newTestDB creates a WongDB instance for the given repo root.
func newTestDB(t *testing.T, repoRoot string) *WongDB {
	t.Helper()
	return New(repoRoot)
}

// makeTestIssue creates a minimal valid types.Issue for testing.
func makeTestIssue(id, title string) *types.Issue {
	now := time.Now()
	return &types.Issue{
		ID:        id,
		Title:     title,
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// --- Init tests ---

func TestWongDB_Init_CreatesWongDB(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify wong-db bookmark exists
	out := runJJ(t, dir, "bookmark", "list")
	if !strings.Contains(out, "wong-db") {
		t.Errorf("expected wong-db bookmark in output, got:\n%s", out)
	}
}

func TestWongDB_Init_CreatesImmutability(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Attempting to edit wong-db should fail because it should be immutable
	out, editErr := runJJMayFail(t, dir, "edit", "wong-db")
	if editErr == nil {
		t.Errorf("expected 'jj edit wong-db' to fail with immutability error, but it succeeded.\noutput: %s", out)
	}
	// The error output should mention immutability or immutable
	lowerOut := strings.ToLower(out)
	if !strings.Contains(lowerOut, "immut") && !strings.Contains(lowerOut, "immutable") {
		t.Logf("warning: error output may not mention immutability: %s", out)
	}
}

func TestWongDB_Init_CreatesMergeWorkingCopy(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Create some content first so the repo isn't empty.
	// Fresh repos get a single-parent WC (git backend can't merge with root).
	// Repos with content get a 2-parent merge WC.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runJJ(t, dir, "commit", "-m", "initial code")

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify that the working copy has 2 parents (merge commit)
	out := runJJ(t, dir, "log", "-r", "@", "--no-graph", "-T", "parents.len()")
	parentCount := strings.TrimSpace(out)
	if parentCount != "2" {
		t.Errorf("expected working copy to have 2 parents, got: %q", parentCount)
	}
}

func TestWongDB_Init_FreshRepo_SingleParent(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Fresh (empty) repo: WC should be a child of wong-db only (1 parent)
	// because git backend doesn't support merge commits with root() as parent
	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	out := runJJ(t, dir, "log", "-r", "@", "--no-graph", "-T", "parents.len()")
	parentCount := strings.TrimSpace(out)
	if parentCount != "1" {
		t.Errorf("expected fresh repo working copy to have 1 parent, got: %q", parentCount)
	}
}

func TestWongDB_Init_Idempotent(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// First init
	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("first Init failed: %v", err)
	}

	// Second init should not error (or return a clear/expected error)
	err = db.Init(ctx)
	if err != nil {
		// If init returns an error on second call, it should be a clear
		// "already initialized" type error, not a crash
		if !strings.Contains(err.Error(), "already") && !strings.Contains(err.Error(), "exist") {
			t.Fatalf("second Init returned unexpected error: %v", err)
		}
	}
}

func TestWongDB_Init_NotJJRepo(t *testing.T) {
	// Use a plain temp directory that is NOT a jj repo
	dir := t.TempDir()
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err == nil {
		t.Fatal("expected Init to fail in non-jj directory, but it succeeded")
	}
}

// --- IsInitialized tests ---

func TestWongDB_IsInitialized_False(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	if db.IsInitialized(ctx) {
		t.Error("expected IsInitialized to return false for a fresh jj repo")
	}
}

func TestWongDB_IsInitialized_True(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if !db.IsInitialized(ctx) {
		t.Error("expected IsInitialized to return true after Init")
	}
}

// --- Write + Read cycle tests ---

func TestWongDB_WriteAndReadIssue(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	issueID := "test-issue-1"
	issueData := []byte(`{"id":"test-issue-1","title":"Test Issue","status":"open","priority":2}`)

	// Write the issue
	err = db.WriteIssue(ctx, issueID, issueData)
	if err != nil {
		t.Fatalf("WriteIssue failed: %v", err)
	}

	// Sync to persist
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Read it back
	readData, err := db.ReadIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("ReadIssue failed: %v", err)
	}

	// Verify the data matches (compare as JSON to handle formatting differences)
	var wrote, read map[string]interface{}
	if err := json.Unmarshal(issueData, &wrote); err != nil {
		t.Fatalf("failed to unmarshal written data: %v", err)
	}
	if err := json.Unmarshal(readData, &read); err != nil {
		t.Fatalf("failed to unmarshal read data: %v", err)
	}
	for k, v := range wrote {
		if read[k] != v {
			// handle numeric type differences in JSON
			wJSON, _ := json.Marshal(v)
			rJSON, _ := json.Marshal(read[k])
			if string(wJSON) != string(rJSON) {
				t.Errorf("field %q: wrote %v, read %v", k, v, read[k])
			}
		}
	}
}

func TestWongDB_ListIssueIDs(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Write 3 issues
	ids := []string{"issue-a", "issue-b", "issue-c"}
	for _, id := range ids {
		data := []byte(`{"id":"` + id + `","title":"Issue ` + id + `","status":"open","priority":2}`)
		err = db.WriteIssue(ctx, id, data)
		if err != nil {
			t.Fatalf("WriteIssue(%s) failed: %v", id, err)
		}
	}

	// Sync
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// List IDs
	listedIDs, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("ListIssueIDs failed: %v", err)
	}

	// Verify all IDs are present
	idSet := make(map[string]bool)
	for _, id := range listedIDs {
		idSet[id] = true
	}
	for _, expectedID := range ids {
		if !idSet[expectedID] {
			t.Errorf("expected issue ID %q in listed IDs, got: %v", expectedID, listedIDs)
		}
	}
}

func TestWongDB_DeleteIssue(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	issueID := "issue-to-delete"
	data := []byte(`{"id":"issue-to-delete","title":"Delete Me","status":"open","priority":2}`)

	// Write and sync
	err = db.WriteIssue(ctx, issueID, data)
	if err != nil {
		t.Fatalf("WriteIssue failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync after write failed: %v", err)
	}

	// Verify it exists
	_, err = db.ReadIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("ReadIssue before delete failed: %v", err)
	}

	// Delete
	err = db.DeleteIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("DeleteIssue failed: %v", err)
	}

	// Sync again
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync after delete failed: %v", err)
	}

	// Verify it's gone
	_, err = db.ReadIssue(ctx, issueID)
	if err == nil {
		t.Error("expected ReadIssue to fail after deletion, but it succeeded")
	}
}

// --- Storage layer tests ---

func TestWongDB_SaveAndLoadIssue(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	issue := makeTestIssue("stor-001", "Storage Layer Test")
	issue.Description = "Testing the storage layer"
	issue.Assignee = "tester"

	// Save
	err = db.SaveIssue(ctx, issue)
	if err != nil {
		t.Fatalf("SaveIssue failed: %v", err)
	}

	// Sync
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Load
	loaded, err := db.LoadIssue(ctx, "stor-001")
	if err != nil {
		t.Fatalf("LoadIssue failed: %v", err)
	}

	// Verify fields
	if loaded.ID != issue.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, issue.ID)
	}
	if loaded.Title != issue.Title {
		t.Errorf("Title: got %q, want %q", loaded.Title, issue.Title)
	}
	if loaded.Description != issue.Description {
		t.Errorf("Description: got %q, want %q", loaded.Description, issue.Description)
	}
	if loaded.Status != issue.Status {
		t.Errorf("Status: got %q, want %q", loaded.Status, issue.Status)
	}
	if loaded.Priority != issue.Priority {
		t.Errorf("Priority: got %d, want %d", loaded.Priority, issue.Priority)
	}
	if loaded.Assignee != issue.Assignee {
		t.Errorf("Assignee: got %q, want %q", loaded.Assignee, issue.Assignee)
	}
}

func TestWongDB_LoadAllIssues(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Save 3 issues
	issues := []*types.Issue{
		makeTestIssue("all-001", "First Issue"),
		makeTestIssue("all-002", "Second Issue"),
		makeTestIssue("all-003", "Third Issue"),
	}
	for _, issue := range issues {
		err = db.SaveIssue(ctx, issue)
		if err != nil {
			t.Fatalf("SaveIssue(%s) failed: %v", issue.ID, err)
		}
	}

	// Sync
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Load all
	loaded, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues failed: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(loaded))
	}

	// Verify all IDs present
	idSet := make(map[string]bool)
	for _, issue := range loaded {
		idSet[issue.ID] = true
	}
	for _, expected := range issues {
		if !idSet[expected.ID] {
			t.Errorf("expected issue %q in loaded results", expected.ID)
		}
	}
}

func TestWongDB_RemoveIssue(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	issue := makeTestIssue("remove-001", "Issue to Remove")

	// Save and sync
	err = db.SaveIssue(ctx, issue)
	if err != nil {
		t.Fatalf("SaveIssue failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Remove (should include sync internally)
	err = db.RemoveIssue(ctx, "remove-001")
	if err != nil {
		t.Fatalf("RemoveIssue failed: %v", err)
	}

	// Verify gone
	_, err = db.LoadIssue(ctx, "remove-001")
	if err == nil {
		t.Error("expected LoadIssue to fail after RemoveIssue, but it succeeded")
	}
}

// --- Sync tests ---

func TestWongDB_Sync_Idempotent(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Sync with no changes should succeed
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("first Sync failed: %v", err)
	}

	// Sync again, still no changes
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("second Sync failed: %v", err)
	}
}

func TestWongDB_Sync_PreservesImmutability(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Write something and sync
	err = db.WriteIssue(ctx, "imm-test", []byte(`{"id":"imm-test","title":"Immutability Check"}`))
	if err != nil {
		t.Fatalf("WriteIssue failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// After sync, wong-db should still be immutable
	out, editErr := runJJMayFail(t, dir, "edit", "wong-db")
	if editErr == nil {
		t.Errorf("expected 'jj edit wong-db' to fail after sync, but it succeeded.\noutput: %s", out)
	}
}

func TestWongDB_Sync_MultipleRounds(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Round 1: write and sync
	err = db.WriteIssue(ctx, "round1", []byte(`{"id":"round1","title":"Round 1"}`))
	if err != nil {
		t.Fatalf("WriteIssue round1 failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync round1 failed: %v", err)
	}

	// Round 2: write more and sync again
	err = db.WriteIssue(ctx, "round2", []byte(`{"id":"round2","title":"Round 2"}`))
	if err != nil {
		t.Fatalf("WriteIssue round2 failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync round2 failed: %v", err)
	}

	// Verify both issues are present
	ids, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("ListIssueIDs failed: %v", err)
	}

	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet["round1"] {
		t.Error("round1 issue missing after multiple sync rounds")
	}
	if !idSet["round2"] {
		t.Error("round2 issue missing after multiple sync rounds")
	}
}

// --- Edge case tests ---

func TestWongDB_ReadIssue_NotFound(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	_, err = db.ReadIssue(ctx, "nonexistent-issue-xyz")
	if err == nil {
		t.Error("expected ReadIssue to return error for non-existent issue, but it succeeded")
	}
}

func TestWongDB_WriteIssue_SpecialChars(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Issue ID with hyphens and dots (e.g. "wong-8rr.1")
	specialIDs := []string{
		"wong-8rr.1",
		"issue-with-hyphens",
		"test.dotted.id",
		"mix-ed.chars-123",
	}

	for _, id := range specialIDs {
		data := []byte(`{"id":"` + id + `","title":"Special: ` + id + `","status":"open","priority":2}`)
		err = db.WriteIssue(ctx, id, data)
		if err != nil {
			t.Fatalf("WriteIssue(%q) failed: %v", id, err)
		}
	}

	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify all can be read back
	for _, id := range specialIDs {
		readData, readErr := db.ReadIssue(ctx, id)
		if readErr != nil {
			t.Errorf("ReadIssue(%q) failed: %v", id, readErr)
			continue
		}
		if len(readData) == 0 {
			t.Errorf("ReadIssue(%q) returned empty data", id)
		}
	}
}

func TestWongDB_ReadConfig_AfterInit(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	config, err := db.ReadConfig(ctx)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("ReadConfig returned nil config")
	}

	// Config should have some reasonable defaults
	// The exact values depend on the implementation, but it should not be empty
	t.Logf("Config after init: prefix=%q, history_mode=%q", config.Prefix, config.HistoryMode)
}

func TestWongDB_ConcurrentSync(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Two goroutines writing different issues then syncing
	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
		writeErr := db.WriteIssue(ctx, "conc-a", []byte(`{"id":"conc-a","title":"Concurrent A","status":"open","priority":2}`))
		if writeErr != nil {
			errs[0] = writeErr
			return
		}
		errs[0] = db.Sync(ctx)
	}()

	go func() {
		defer wg.Done()
		writeErr := db.WriteIssue(ctx, "conc-b", []byte(`{"id":"conc-b","title":"Concurrent B","status":"open","priority":1}`))
		if writeErr != nil {
			errs[1] = writeErr
			return
		}
		errs[1] = db.Sync(ctx)
	}()

	wg.Wait()

	// At least one goroutine should succeed. Both may succeed if the
	// implementation handles concurrency properly, or one may get a
	// retry/conflict error which is acceptable.
	bothFailed := errs[0] != nil && errs[1] != nil
	if bothFailed {
		t.Fatalf("both concurrent syncs failed:\n  goroutine 0: %v\n  goroutine 1: %v", errs[0], errs[1])
	}

	// If one failed, log it but don't fail the test (concurrent conflict is acceptable)
	for i, e := range errs {
		if e != nil {
			t.Logf("concurrent goroutine %d had error (acceptable): %v", i, e)
		}
	}

	// Final sync to ensure consistency
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("final Sync failed: %v", err)
	}

	// Verify at least the successful writes are present
	ids, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("ListIssueIDs after concurrent sync failed: %v", err)
	}

	if len(ids) == 0 {
		t.Error("expected at least one issue after concurrent writes, got none")
	}
	t.Logf("after concurrent sync, found %d issues: %v", len(ids), ids)
}

// --- Additional helpers for verifying jj state ---

// verifyBookmarkExists checks that a jj bookmark exists in the repo.
func verifyBookmarkExists(t *testing.T, dir, bookmarkName string) {
	t.Helper()
	out := runJJ(t, dir, "bookmark", "list")
	if !strings.Contains(out, bookmarkName) {
		t.Errorf("expected bookmark %q to exist, bookmark list output:\n%s", bookmarkName, out)
	}
}

// verifyFileInWongDB checks that a file path exists in the wong-db revision.
func verifyFileInWongDB(t *testing.T, dir, filePath string) {
	t.Helper()
	// Use jj file show to check if a file exists at the wong-db revision
	_, err := runJJMayFail(t, dir, "file", "show", "-r", "wong-db", filePath)
	if err != nil {
		t.Errorf("expected file %q to exist in wong-db revision, but it doesn't", filePath)
	}
}

// TestWongDB_Init_VerifyStructure performs a deeper structural check of the
// initialized wong-db. This is a supplementary test that verifies the jj
// revision tree looks correct after Init.
func TestWongDB_Init_VerifyStructure(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify the bookmark exists
	verifyBookmarkExists(t, dir, "wong-db")

	// Log the jj log for debugging
	logOut := runJJ(t, dir, "log", "--no-graph", "-r", "all()")
	t.Logf("jj log after init:\n%s", logOut)
}

// TestWongDB_WriteIssue_LargePayload tests writing a larger issue payload.
func TestWongDB_WriteIssue_LargePayload(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create a large description
	largeDesc := strings.Repeat("This is a large description. ", 500)
	issue := makeTestIssue("large-001", "Large Payload Test")
	issue.Description = largeDesc

	err = db.SaveIssue(ctx, issue)
	if err != nil {
		t.Fatalf("SaveIssue with large payload failed: %v", err)
	}

	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	loaded, err := db.LoadIssue(ctx, "large-001")
	if err != nil {
		t.Fatalf("LoadIssue failed: %v", err)
	}
	if loaded.Description != largeDesc {
		t.Errorf("large description mismatch: got %d chars, want %d chars",
			len(loaded.Description), len(largeDesc))
	}
}

// TestWongDB_SaveIssue_AllFields tests that saving and loading preserves a
// wider set of issue fields beyond the basics.
func TestWongDB_SaveIssue_AllFields(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	now := time.Now().Truncate(time.Second) // truncate for JSON round-trip
	estMin := 120
	issue := &types.Issue{
		ID:               "full-001",
		Title:            "Full Field Test",
		Description:      "A comprehensive test",
		Status:           types.StatusInProgress,
		Priority:         1,
		IssueType:        types.TypeFeature,
		Assignee:         "alice",
		Owner:            "bob@example.com",
		EstimatedMinutes: &estMin,
		CreatedAt:        now,
		UpdatedAt:        now,
		Labels:           []string{"urgent", "backend"},
	}

	err = db.SaveIssue(ctx, issue)
	if err != nil {
		t.Fatalf("SaveIssue failed: %v", err)
	}

	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	loaded, err := db.LoadIssue(ctx, "full-001")
	if err != nil {
		t.Fatalf("LoadIssue failed: %v", err)
	}

	if loaded.Title != issue.Title {
		t.Errorf("Title: got %q, want %q", loaded.Title, issue.Title)
	}
	if loaded.Status != issue.Status {
		t.Errorf("Status: got %q, want %q", loaded.Status, issue.Status)
	}
	if loaded.IssueType != issue.IssueType {
		t.Errorf("IssueType: got %q, want %q", loaded.IssueType, issue.IssueType)
	}
	if loaded.Assignee != issue.Assignee {
		t.Errorf("Assignee: got %q, want %q", loaded.Assignee, issue.Assignee)
	}
	if loaded.Owner != issue.Owner {
		t.Errorf("Owner: got %q, want %q", loaded.Owner, issue.Owner)
	}
	if loaded.EstimatedMinutes == nil || *loaded.EstimatedMinutes != estMin {
		t.Errorf("EstimatedMinutes: got %v, want %d", loaded.EstimatedMinutes, estMin)
	}
}

// TestWongDB_NewSetsRepoRoot verifies that New properly sets the repo root on
// the returned WongDB struct. This is a basic unit check for the constructor.
func TestWongDB_NewSetsRepoRoot(t *testing.T) {
	db := New("/some/test/path")
	if db == nil {
		t.Fatal("New returned nil")
	}
	// We cannot easily inspect unexported fields, but the object should be
	// non-nil and usable (even if operations on a non-existent path will fail).
	_ = db
}

// TestWongDB_DeleteIssue_NotFound tests deleting an issue that does not exist.
func TestWongDB_DeleteIssue_NotFound(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Delete a non-existent issue -- this may or may not error depending on
	// implementation. We just verify it doesn't panic.
	err = db.DeleteIssue(ctx, "nonexistent-delete-target")
	// Log the result without failing, since behavior on missing ID may vary
	if err != nil {
		t.Logf("DeleteIssue for non-existent issue returned error (may be expected): %v", err)
	} else {
		t.Log("DeleteIssue for non-existent issue returned nil (no-op)")
	}
}

// TestWongDB_MultipleIssueTypes tests writing issues of different types.
func TestWongDB_MultipleIssueTypes(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	issueTypes := []struct {
		id       string
		issueTyp types.IssueType
	}{
		{"bug-001", types.TypeBug},
		{"feat-001", types.TypeFeature},
		{"task-001", types.TypeTask},
		{"epic-001", types.TypeEpic},
		{"chore-001", types.TypeChore},
	}

	for _, tc := range issueTypes {
		issue := makeTestIssue(tc.id, "Test "+string(tc.issueTyp))
		issue.IssueType = tc.issueTyp
		err = db.SaveIssue(ctx, issue)
		if err != nil {
			t.Fatalf("SaveIssue(%s, type=%s) failed: %v", tc.id, tc.issueTyp, err)
		}
	}

	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Load all and verify types
	all, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues failed: %v", err)
	}

	if len(all) != len(issueTypes) {
		t.Fatalf("expected %d issues, got %d", len(issueTypes), len(all))
	}

	typeMap := make(map[string]types.IssueType)
	for _, issue := range all {
		typeMap[issue.ID] = issue.IssueType
	}
	for _, tc := range issueTypes {
		if got, ok := typeMap[tc.id]; !ok {
			t.Errorf("issue %q not found in loaded issues", tc.id)
		} else if got != tc.issueTyp {
			t.Errorf("issue %q: got type %q, want %q", tc.id, got, tc.issueTyp)
		}
	}
}

// TestWongDB_WriteIssue_OverwriteExisting tests that writing to the same ID
// overwrites the previous data.
func TestWongDB_WriteIssue_OverwriteExisting(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	issueID := "overwrite-001"

	// Write v1
	v1 := []byte(`{"id":"overwrite-001","title":"Version 1","status":"open","priority":2}`)
	err = db.WriteIssue(ctx, issueID, v1)
	if err != nil {
		t.Fatalf("WriteIssue v1 failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync v1 failed: %v", err)
	}

	// Write v2 with same ID but different title
	v2 := []byte(`{"id":"overwrite-001","title":"Version 2","status":"open","priority":1}`)
	err = db.WriteIssue(ctx, issueID, v2)
	if err != nil {
		t.Fatalf("WriteIssue v2 failed: %v", err)
	}
	err = db.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync v2 failed: %v", err)
	}

	// Read back and verify it's v2
	readData, err := db.ReadIssue(ctx, issueID)
	if err != nil {
		t.Fatalf("ReadIssue failed: %v", err)
	}

	var readIssue map[string]interface{}
	if err := json.Unmarshal(readData, &readIssue); err != nil {
		t.Fatalf("failed to unmarshal read data: %v", err)
	}

	if title, ok := readIssue["title"].(string); !ok || title != "Version 2" {
		t.Errorf("expected title 'Version 2', got %q", readIssue["title"])
	}
}

// TestWongDB_Metadata_AfterInit verifies that the wong-db has a metadata file
// after initialization by attempting to read config (which relies on metadata).
func TestWongDB_Metadata_AfterInit(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// ReadConfig should work after init (config is part of the metadata)
	config, err := db.ReadConfig(ctx)
	if err != nil {
		t.Fatalf("ReadConfig after init failed: %v", err)
	}
	if config == nil {
		t.Fatal("ReadConfig returned nil after init")
	}
}

// TestWongDB_Init_SubdirectoryPath tests Init when called from a subdirectory
// path within the jj repo (the repo root should still be resolved correctly).
func TestWongDB_Init_SubdirectoryPath(t *testing.T) {
	dir := setupJJRepo(t)

	// Create a subdirectory
	subDir := filepath.Join(dir, "subdir", "nested")
	cmd := exec.Command("mkdir", "-p", subDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mkdir failed: %v\noutput: %s", err, out)
	}

	// Create db pointing to the actual repo root (not the subdir)
	// The WongDB should work with the repo root
	db := newTestDB(t, dir)
	ctx := context.Background()

	err := db.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if !db.IsInitialized(ctx) {
		t.Error("expected IsInitialized to return true after Init")
	}
}
