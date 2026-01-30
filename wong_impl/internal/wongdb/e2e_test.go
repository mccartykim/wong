package wongdb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// TestE2E_FullWorkflow exercises the complete user journey:
// init -> create issues -> sync -> list -> update -> delete -> verify.
func TestE2E_FullWorkflow(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Step 1: jj git init already done by setupJJRepo

	// Step 2: wong init
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if !db.IsInitialized(ctx) {
		t.Fatal("expected IsInitialized to return true after Init")
	}

	// Step 3: Create 3 issues
	issues := []*types.Issue{
		makeTestIssue("e2e-001", "First issue"),
		makeTestIssue("e2e-002", "Second issue"),
		makeTestIssue("e2e-003", "Third issue"),
	}
	for _, issue := range issues {
		if err := db.SaveIssue(ctx, issue); err != nil {
			t.Fatalf("SaveIssue(%s) failed: %v", issue.ID, err)
		}
	}

	// Step 4: Sync
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Step 5: List all issues - verify 3
	loaded, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(loaded))
	}

	// Step 6: Close one issue
	toClose, err := db.LoadIssue(ctx, "e2e-001")
	if err != nil {
		t.Fatalf("LoadIssue(e2e-001) failed: %v", err)
	}
	now := time.Now()
	toClose.Status = types.StatusClosed
	toClose.ClosedAt = &now
	toClose.UpdatedAt = now
	if err := db.SaveIssue(ctx, toClose); err != nil {
		t.Fatalf("SaveIssue (close) failed: %v", err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync after close failed: %v", err)
	}

	// Step 7: Verify updated issue has new status
	reloaded, err := db.LoadIssue(ctx, "e2e-001")
	if err != nil {
		t.Fatalf("LoadIssue after close failed: %v", err)
	}
	if reloaded.Status != types.StatusClosed {
		t.Errorf("expected status %q, got %q", types.StatusClosed, reloaded.Status)
	}
	if reloaded.ClosedAt == nil {
		t.Error("expected ClosedAt to be set after closing")
	}

	// Step 8: Delete one issue
	if err := db.RemoveIssue(ctx, "e2e-002"); err != nil {
		t.Fatalf("RemoveIssue(e2e-002) failed: %v", err)
	}

	// Step 9: List - verify 2 remaining
	remaining, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues after delete failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining issues, got %d", len(remaining))
	}
	idSet := make(map[string]bool)
	for _, issue := range remaining {
		idSet[issue.ID] = true
	}
	if idSet["e2e-002"] {
		t.Error("deleted issue e2e-002 should not be in remaining list")
	}
	if !idSet["e2e-001"] || !idSet["e2e-003"] {
		t.Errorf("expected e2e-001 and e2e-003 in remaining, got: %v", idSet)
	}
}

// TestE2E_InitWithExistingContent verifies that wong init works correctly
// in a repo that already has committed code, and that wong-db is isolated
// from existing code files.
func TestE2E_InitWithExistingContent(t *testing.T) {
	dir := setupJJRepo(t)
	ctx := context.Background()

	// Step 1: Create repo with existing files and committed content
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# My Project\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runJJ(t, dir, "commit", "-m", "initial project files")

	// Step 2: wong init
	db := newTestDB(t, dir)
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Step 3: Verify existing files still accessible in working copy
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Errorf("main.go should still be accessible in working copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Errorf("README.md should still be accessible in working copy: %v", err)
	}

	// Step 4: Verify wong-db exists with .wong/ files
	verifyBookmarkExists(t, dir, "wong-db")
	verifyFileInWongDB(t, dir, ".wong/config.json")
	verifyFileInWongDB(t, dir, ".wong/metadata.json")

	// Step 5: Create an issue, sync, verify it's on wong-db
	issue := makeTestIssue("exist-001", "Issue in existing repo")
	if err := db.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("SaveIssue failed: %v", err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
	verifyFileInWongDB(t, dir, ".wong/issues/exist-001.json")

	// Step 6: Verify existing code files NOT on wong-db
	out, err := runJJMayFail(t, dir, "file", "list", "-r", "wong-db")
	if err != nil {
		t.Fatalf("file list on wong-db failed: %v", err)
	}
	if strings.Contains(out, "main.go") {
		t.Error("main.go should NOT be on wong-db revision")
	}
	if strings.Contains(out, "README.md") {
		t.Error("README.md should NOT be on wong-db revision")
	}
}

// TestE2E_SyncAfterJJOperations simulates real jj workflow interleaved with
// wong issue tracking: creating jj changes for feature work alongside issue ops.
func TestE2E_SyncAfterJJOperations(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Step 1: Init wong
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Step 2: Create issue
	issue1 := makeTestIssue("jjop-001", "First tracked issue")
	if err := db.SaveIssue(ctx, issue1); err != nil {
		t.Fatalf("SaveIssue(jjop-001) failed: %v", err)
	}

	// Step 3: Sync
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync after first issue failed: %v", err)
	}

	// Step 4: jj new (create new change for feature work)
	runJJ(t, dir, "new", "-m", "feature: add utils module")

	// Step 5: Add code files to the new change
	utilsDir := filepath.Join(dir, "utils")
	if err := os.MkdirAll(utilsDir, 0o755); err != nil {
		t.Fatalf("mkdir utils: %v", err)
	}
	if err := os.WriteFile(filepath.Join(utilsDir, "helpers.go"),
		[]byte("package utils\n\nfunc Helper() string { return \"help\" }\n"), 0o644); err != nil {
		t.Fatalf("write helpers.go: %v", err)
	}

	// Step 6: Create another issue
	issue2 := makeTestIssue("jjop-002", "Second tracked issue")
	if err := db.SaveIssue(ctx, issue2); err != nil {
		t.Fatalf("SaveIssue(jjop-002) failed: %v", err)
	}

	// Step 7: Sync - verify both issues on wong-db, code NOT on wong-db
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync after jj operations failed: %v", err)
	}

	// Both issues should be on wong-db
	verifyFileInWongDB(t, dir, ".wong/issues/jjop-001.json")
	verifyFileInWongDB(t, dir, ".wong/issues/jjop-002.json")

	// Code should NOT be on wong-db
	wongDBFiles, err := runJJMayFail(t, dir, "file", "list", "-r", "wong-db")
	if err != nil {
		t.Fatalf("file list wong-db failed: %v", err)
	}
	if strings.Contains(wongDBFiles, "helpers.go") {
		t.Error("helpers.go should NOT be on wong-db")
	}
	if strings.Contains(wongDBFiles, "utils/") {
		t.Error("utils/ directory should NOT be on wong-db")
	}

	// Verify both issues are loadable
	allIssues, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues failed: %v", err)
	}
	if len(allIssues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(allIssues))
	}
}

// TestE2E_WongDBIsolation verifies that wong-db data is isolated from
// normal code commits and vice versa.
func TestE2E_WongDBIsolation(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Step 1: Init wong
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Step 2: Save 3 issues, sync
	for i := 1; i <= 3; i++ {
		issue := makeTestIssue(
			"iso-00"+string(rune('0'+i)),
			"Isolation test issue "+string(rune('0'+i)),
		)
		if err := db.SaveIssue(ctx, issue); err != nil {
			t.Fatalf("SaveIssue failed: %v", err)
		}
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Step 3: Create code files in working copy
	if err := os.WriteFile(filepath.Join(dir, "app.go"),
		[]byte("package main\n\nfunc App() {}\n"), 0o644); err != nil {
		t.Fatalf("write app.go: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "lib.go"),
		[]byte("package pkg\n\nfunc Lib() {}\n"), 0o644); err != nil {
		t.Fatalf("write lib.go: %v", err)
	}

	// Step 4: Commit code with jj commit
	runJJ(t, dir, "commit", "-m", "feature: add app and pkg")

	// Step 5: Verify that the code files are visible in the working copy
	// (proving code and wong coexist) but that wong-db's own tree is pure.
	// In the merge architecture, the working copy merges code and wong-db,
	// so committed code changes inherit .wong/ from wong-db parent. The key
	// isolation invariant is that wong-db itself has no code files.
	wcFiles := runJJ(t, dir, "file", "list", "-r", "@-")
	if !strings.Contains(wcFiles, "app.go") {
		t.Error("app.go should be visible in the committed code change")
	}
	if !strings.Contains(wcFiles, "pkg/lib.go") {
		t.Error("pkg/lib.go should be visible in the committed code change")
	}

	// Step 6: Verify wong-db does NOT contain code files
	wongFiles, wongErr := runJJMayFail(t, dir, "file", "list", "-r", "wong-db")
	if wongErr != nil {
		t.Fatalf("file list wong-db failed: %v", wongErr)
	}
	if strings.Contains(wongFiles, "app.go") {
		t.Error("wong-db should NOT contain app.go")
	}
	if strings.Contains(wongFiles, "pkg/") {
		t.Error("wong-db should NOT contain pkg/")
	}

	// Verify wong-db DOES have issue files
	if !strings.Contains(wongFiles, ".wong/") {
		t.Error("wong-db should contain .wong/ directory")
	}
}

// TestE2E_ImmutabilityProtection verifies that wong-db is immutable to
// regular jj operations but still accessible to wong sync (via --config override).
func TestE2E_ImmutabilityProtection(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Step 1: Init wong
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Step 2: Try jj edit wong-db - should fail
	editOut, editErr := runJJMayFail(t, dir, "edit", "wong-db")
	if editErr == nil {
		t.Errorf("expected 'jj edit wong-db' to fail, but it succeeded. output: %s", editOut)
	} else {
		lowerOut := strings.ToLower(editOut)
		if !strings.Contains(lowerOut, "immut") {
			t.Logf("edit error output (expected immutability mention): %s", editOut)
		}
	}

	// Step 3: Try jj describe wong-db - should fail
	descOut, descErr := runJJMayFail(t, dir, "describe", "wong-db", "-m", "hack")
	if descErr == nil {
		t.Errorf("expected 'jj describe wong-db' to fail, but it succeeded. output: %s", descOut)
	} else {
		lowerOut := strings.ToLower(descOut)
		if !strings.Contains(lowerOut, "immut") {
			t.Logf("describe error output (expected immutability mention): %s", descOut)
		}
	}

	// Step 4: Verify wong CLI sync still works (bypasses via --config)
	issue := makeTestIssue("immut-001", "Created despite immutability")
	if err := db.SaveIssue(ctx, issue); err != nil {
		t.Fatalf("SaveIssue failed: %v", err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync failed (should bypass immutability): %v", err)
	}

	// Verify the issue is on wong-db
	loaded, err := db.LoadIssue(ctx, "immut-001")
	if err != nil {
		t.Fatalf("LoadIssue after sync-through-immutability failed: %v", err)
	}
	if loaded.Title != "Created despite immutability" {
		t.Errorf("expected title %q, got %q", "Created despite immutability", loaded.Title)
	}
}

// TestE2E_MultipleIssueLifecycle exercises full CRUD with realistic issue data:
// epic + subtasks, comments, status transitions, and final verification.
func TestE2E_MultipleIssueLifecycle(t *testing.T) {
	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Step 1: Init
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Step 2: Create epic issue
	epic := makeTestIssue("lifecycle-epic", "Build authentication system")
	epic.IssueType = types.TypeEpic
	epic.Description = "Implement complete auth flow with login, registration, and password reset."
	epic.Priority = 1
	epic.Assignee = "alice"
	if err := db.SaveIssue(ctx, epic); err != nil {
		t.Fatalf("SaveIssue(epic) failed: %v", err)
	}

	// Step 3: Create 3 subtask issues
	subtasks := []*types.Issue{
		makeTestIssue("lifecycle-sub1", "Implement login endpoint"),
		makeTestIssue("lifecycle-sub2", "Implement registration form"),
		makeTestIssue("lifecycle-sub3", "Add password reset flow"),
	}
	for i, sub := range subtasks {
		sub.IssueType = types.TypeTask
		sub.Assignee = "bob"
		sub.Priority = 2
		sub.Description = "Subtask " + string(rune('1'+i)) + " of auth epic"
		if err := db.SaveIssue(ctx, sub); err != nil {
			t.Fatalf("SaveIssue(%s) failed: %v", sub.ID, err)
		}
	}

	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync after creation failed: %v", err)
	}

	// Step 4: Add comments to subtasks
	for _, sub := range subtasks {
		loaded, err := db.LoadIssue(ctx, sub.ID)
		if err != nil {
			t.Fatalf("LoadIssue(%s) for comments failed: %v", sub.ID, err)
		}
		loaded.Comments = append(loaded.Comments, &types.Comment{
			ID:        1,
			IssueID:   loaded.ID,
			Author:    "alice",
			Text:      "Starting work on " + loaded.Title,
			CreatedAt: time.Now(),
		})
		loaded.Comments = append(loaded.Comments, &types.Comment{
			ID:        2,
			IssueID:   loaded.ID,
			Author:    "bob",
			Text:      "Acknowledged, will begin shortly.",
			CreatedAt: time.Now(),
		})
		loaded.UpdatedAt = time.Now()
		if err := db.SaveIssue(ctx, loaded); err != nil {
			t.Fatalf("SaveIssue(%s) with comments failed: %v", loaded.ID, err)
		}
	}

	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync after comments failed: %v", err)
	}

	// Verify comments persisted
	withComments, err := db.LoadIssue(ctx, "lifecycle-sub1")
	if err != nil {
		t.Fatalf("LoadIssue for comment check failed: %v", err)
	}
	if len(withComments.Comments) != 2 {
		t.Errorf("expected 2 comments on lifecycle-sub1, got %d", len(withComments.Comments))
	}

	// Step 5: Close subtasks one by one
	now := time.Now()
	for _, sub := range subtasks {
		loaded, err := db.LoadIssue(ctx, sub.ID)
		if err != nil {
			t.Fatalf("LoadIssue(%s) for closing failed: %v", sub.ID, err)
		}
		loaded.Status = types.StatusClosed
		loaded.ClosedAt = &now
		loaded.UpdatedAt = now
		if err := db.SaveIssue(ctx, loaded); err != nil {
			t.Fatalf("SaveIssue(%s) close failed: %v", loaded.ID, err)
		}
		if err := db.Sync(ctx); err != nil {
			t.Fatalf("Sync after closing %s failed: %v", loaded.ID, err)
		}
	}

	// Step 6: Verify only epic remains open
	allIssues, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues failed: %v", err)
	}
	openCount := 0
	for _, issue := range allIssues {
		if issue.Status == types.StatusOpen {
			openCount++
			if issue.ID != "lifecycle-epic" {
				t.Errorf("expected only epic to be open, but %s is open", issue.ID)
			}
		}
	}
	if openCount != 1 {
		t.Errorf("expected exactly 1 open issue (epic), got %d", openCount)
	}

	// Step 7: Close epic
	epicLoaded, err := db.LoadIssue(ctx, "lifecycle-epic")
	if err != nil {
		t.Fatalf("LoadIssue(epic) for closing failed: %v", err)
	}
	epicLoaded.Status = types.StatusClosed
	epicLoaded.ClosedAt = &now
	epicLoaded.UpdatedAt = now
	if err := db.SaveIssue(ctx, epicLoaded); err != nil {
		t.Fatalf("SaveIssue(epic) close failed: %v", err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Sync after closing epic failed: %v", err)
	}

	// Step 8: LoadAllIssues - all should be closed status
	finalIssues, err := db.LoadAllIssues(ctx)
	if err != nil {
		t.Fatalf("LoadAllIssues (final) failed: %v", err)
	}
	if len(finalIssues) != 4 {
		t.Fatalf("expected 4 total issues, got %d", len(finalIssues))
	}
	for _, issue := range finalIssues {
		if issue.Status != types.StatusClosed {
			t.Errorf("expected all issues to be closed, but %s has status %q", issue.ID, issue.Status)
		}
	}
}
