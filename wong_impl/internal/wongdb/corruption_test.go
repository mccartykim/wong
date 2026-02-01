package wongdb

import (
	"context"
	"os"
	"testing"
)

// Tests for error detection helpers (wong-q3 fix).
// These don't require jj to be installed.

func TestIsStaleWorkingCopyError(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name:   "exact jj stale message",
			stderr: "The working copy is stale (run `jj workspace update-stale` to recover)\n",
			want:   true,
		},
		{
			name:   "lowercase variant",
			stderr: "working copy is stale\n",
			want:   true,
		},
		{
			name:   "stale message with extra context",
			stderr: "The working copy is stale (operation abcdef)\nHint: run jj workspace update-stale\n",
			want:   true,
		},
		{
			name:   "stale as substring in unrelated error should not match",
			stderr: "Error: the file 'working copy is stale.txt' was not found\n",
			want:   false,
		},
		{
			name:   "empty stderr",
			stderr: "",
			want:   false,
		},
		{
			name:   "unrelated error",
			stderr: "Error: No such path\n",
			want:   false,
		},
		{
			name:   "stale buried in middle of line should not match",
			stderr: "Error: something The working copy is stale\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStaleWorkingCopyError(tt.stderr)
			if got != tt.want {
				t.Errorf("isStaleWorkingCopyError(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

func TestIsNoChangesError(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		want   bool
	}{
		{
			name:   "Nothing changed with period",
			errMsg: "wongdb: jj squash: exit status 1\nNothing changed.",
			want:   true,
		},
		{
			name:   "no changes to squash",
			errMsg: "wongdb: jj squash: exit status 1\nno changes to squash",
			want:   true,
		},
		{
			name:   "Nothing changed without period should not match",
			errMsg: "wongdb: jj squash: exit status 1\nNothing changed",
			want:   false,
		},
		{
			name:   "unrelated error mentioning changes",
			errMsg: "Error: no such revision",
			want:   false,
		},
		{
			name:   "empty message",
			errMsg: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNoChangesError(tt.errMsg)
			if got != tt.want {
				t.Errorf("isNoChangesError(%q) = %v, want %v", tt.errMsg, got, tt.want)
			}
		})
	}
}

// Tests for corruption handling (wong-q5 fix).

func TestReadyStatus_ZeroValue(t *testing.T) {
	rs := &ReadyStatus{}
	if rs.Ready {
		t.Error("zero-value ReadyStatus should not be ready")
	}
	if len(rs.Blockers) != 0 {
		t.Error("zero-value ReadyStatus should have no blockers")
	}
	if len(rs.Errors) != 0 {
		t.Error("zero-value ReadyStatus should have no errors")
	}
}

func TestWongDB_WriteIssue_DirtyTracking(t *testing.T) {
	dir := t.TempDir()
	db := &WongDB{repoRoot: dir, jjBin: "jj"}

	issuesDir := dir + "/.wong/issues"
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := []byte(`{"id":"test-1","title":"Test Issue"}`)
	ctx := t.Context()
	if err := db.WriteIssue(ctx, "test-1", data); err != nil {
		t.Fatalf("WriteIssue: %v", err)
	}

	// Verify dirty tracking
	db.mu.Lock()
	got := db.dirtyFiles[".wong/issues/test-1.json"]
	db.mu.Unlock()

	if string(got) != string(data) {
		t.Errorf("dirty file content mismatch: got %q, want %q", got, data)
	}
}

func TestWongDB_SnapshotDirtyFiles_Empty(t *testing.T) {
	db := &WongDB{}
	snap := db.snapshotDirtyFiles()
	if snap != nil {
		t.Error("snapshot of empty dirtyFiles should be nil")
	}
}

func TestWongDB_SnapshotDirtyFiles_Returns_Copy(t *testing.T) {
	db := &WongDB{
		dirtyFiles: map[string][]byte{
			"a.json": []byte("hello"),
			"b.json": []byte("world"),
		},
	}

	snap := db.snapshotDirtyFiles()
	if len(snap) != 2 {
		t.Fatalf("snapshot should have 2 entries, got %d", len(snap))
	}

	// Mutating the snapshot should not affect the original
	snap["c.json"] = []byte("new")
	db.mu.Lock()
	if _, ok := db.dirtyFiles["c.json"]; ok {
		t.Error("mutating snapshot should not affect original dirtyFiles")
	}
	db.mu.Unlock()
}

func TestWongDB_RestoreWongFiles(t *testing.T) {
	dir := t.TempDir()
	db := &WongDB{repoRoot: dir}

	files := map[string][]byte{
		".wong/issues/test-1.json": []byte(`{"id":"test-1"}`),
		".wong/issues/test-2.json": []byte(`{"id":"test-2"}`),
	}

	if err := db.restoreWongFiles(files); err != nil {
		t.Fatalf("restoreWongFiles: %v", err)
	}

	// Verify files were written
	for rel, want := range files {
		got, err := os.ReadFile(dir + "/" + rel)
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: got %q, want %q", rel, got, want)
		}
	}
}

func TestWongDB_DirtyTracking_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	db := &WongDB{repoRoot: dir, jjBin: "jj"}

	issuesDir := dir + "/.wong/issues"
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	const n = 50
	done := make(chan error, n)

	for i := 0; i < n; i++ {
		go func(i int) {
			id := "concurrent-" + string(rune('a'+i%26))
			data := []byte(`{"id":"` + id + `"}`)
			done <- db.WriteIssue(ctx, id, data)
		}(i)
	}

	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent WriteIssue failed: %v", err)
		}
	}

	// Verify dirtyFiles map is consistent
	db.mu.Lock()
	count := len(db.dirtyFiles)
	db.mu.Unlock()

	if count == 0 {
		t.Error("expected dirty files after concurrent writes")
	}
}

// Tests for issue ID validation (wong-q10 fix).

func TestValidateIssueID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "valid simple", id: "my-issue-1", wantErr: false},
		{name: "valid with dots", id: "issue.v2", wantErr: false},
		{name: "valid with underscores", id: "bug_fix_123", wantErr: false},
		{name: "empty", id: "", wantErr: true},
		{name: "forward slash", id: "foo/bar", wantErr: true},
		{name: "backslash", id: `foo\bar`, wantErr: true},
		{name: "path traversal dotdot", id: "../etc/passwd", wantErr: true},
		{name: "dotdot in middle", id: "foo..bar", wantErr: true},
		{name: "just dotdot", id: "..", wantErr: true},
		{name: "deep traversal", id: "../../../etc/shadow", wantErr: true},
		{name: "single dot", id: ".", wantErr: true},
		{name: "valid starting with dot", id: ".hidden-issue", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIssueID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIssueID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestWriteIssue_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	db := &WongDB{repoRoot: dir, jjBin: "jj"}

	if err := os.MkdirAll(dir+"/.wong/issues", 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()
	maliciousIDs := []string{
		"../../../etc/passwd",
		"foo/bar",
		`foo\bar`,
		"..",
	}

	for _, id := range maliciousIDs {
		err := db.WriteIssue(ctx, id, []byte(`{"id":"bad"}`))
		if err == nil {
			t.Errorf("WriteIssue(%q) should have returned error", id)
		}
	}

	// Verify no files escaped .wong/issues/
	etcPath := dir + "/etc"
	if _, err := os.Stat(etcPath); err == nil {
		t.Error("path traversal created files outside .wong/issues/")
	}
}

func TestDeleteIssue_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	db := &WongDB{repoRoot: dir, jjBin: "jj"}

	ctx := t.Context()
	err := db.DeleteIssue(ctx, "../../../etc/passwd")
	if err == nil {
		t.Error("DeleteIssue with path traversal should return error")
	}
}

func TestReadIssue_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	// Create a .jj dir so it looks like a jj repo (ReadIssue calls runJJ)
	db := &WongDB{repoRoot: dir, jjBin: "jj"}

	ctx := t.Context()
	_, err := db.ReadIssue(ctx, "../../../etc/passwd")
	if err == nil {
		t.Error("ReadIssue with path traversal should return error")
	}
}

// Tests for Decorator nil safety (wong-q11 fix).

func TestDecorator_NilDB_NoWriteCommand(t *testing.T) {
	d := NewDecorator(nil)
	if d.jjBin != "jj" {
		t.Errorf("expected jjBin 'jj', got %q", d.jjBin)
	}
	// extractSubcommand and isWriteCommand should work fine with nil db
	if d.isWriteCommand("log") {
		t.Error("log should not be a write command")
	}
	if !d.isWriteCommand("commit") {
		t.Error("commit should be a write command")
	}
}

func TestDecorator_NilDB_RunDoesNotPanic(t *testing.T) {
	d := NewDecorator(nil)
	ctx := context.Background()
	// Run with a command that will fail (jj not installed), but should not panic
	// even though db is nil and "new" is a write command
	_ = d.Run(ctx, []string{"new"})
	// If we got here without panicking, the test passes
}
