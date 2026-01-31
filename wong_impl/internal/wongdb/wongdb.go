package wongdb

// WongDB manages issue storage on a dedicated jj change ("wong-db").
// It uses a parallel branch off root() to store per-issue JSON files,
// synced via atomic jj squash with --config override.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// wongDir is the top-level directory for wong data in the working copy.
	wongDir = ".wong"

	// wongIssuesDir is the directory path for issue JSON files.
	wongIssuesDir = ".wong/issues"

	// wongDBBookmark is the jj bookmark name for the wong-db change.
	wongDBBookmark = "wong-db"
)

// WongDB manages issue storage on a dedicated jj change ("wong-db").
type WongDB struct {
	repoRoot string
	jjBin    string
	// dirtyFiles tracks .wong/ files written by this instance that haven't been
	// synced yet. Keys are relative paths (e.g., ".wong/issues/bt-1.json").
	// This is used to preserve pending changes across jj workspace update-stale.
	dirtyFiles map[string][]byte
}

// Config represents .wong/config.yaml (stored as JSON for simplicity).
type Config struct {
	Prefix      string `json:"prefix"`
	HistoryMode string `json:"history_mode"` // "squash" (default) or "chain"
}

// Metadata represents .wong/metadata.json.
type Metadata struct {
	Version   int       `json:"version"`
	Backend   string    `json:"backend"`
	CreatedAt time.Time `json:"created_at"`
}

// New creates a new WongDB instance for the given repo root.
// It auto-detects the jj binary path.
func New(repoRoot string) *WongDB {
	jjBin := "jj"
	if path, err := exec.LookPath("jj"); err == nil {
		jjBin = path
	}
	return &WongDB{
		repoRoot: repoRoot,
		jjBin:    jjBin,
	}
}

// jjCmd creates an exec.Cmd for running jj with the given arguments.
// The command's working directory is set to the repo root.
func (db *WongDB) jjCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, db.jjBin, args...)
	cmd.Dir = db.repoRoot
	return cmd
}

// runJJ executes a jj command and returns its stdout output.
// Returns an error wrapping stderr if the command fails.
// If the command fails due to a stale working copy (common in multi-workspace
// scenarios), it saves .wong/ files, runs `jj workspace update-stale`, restores
// the .wong/ files, and retries the command once. This prevents update-stale
// from overwriting pending .wong/ modifications.
func (db *WongDB) runJJ(ctx context.Context, args ...string) (string, error) {
	cmd := db.jjCmd(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		// Auto-recover from stale working copy (don't retry update-stale itself)
		if strings.Contains(errMsg, "working copy is stale") &&
			!(len(args) >= 2 && args[0] == "workspace" && args[1] == "update-stale") {
			updateCmd := db.jjCmd(ctx, "workspace", "update-stale")
			updateCmd.Run() // best-effort

			// Restore only files that THIS instance wrote (dirty files),
			// which update-stale may have overwritten
			if len(db.dirtyFiles) > 0 {
				db.restoreWongFiles(db.dirtyFiles)
			}

			cmd2 := db.jjCmd(ctx, args...)
			var stdout2, stderr2 bytes.Buffer
			cmd2.Stdout = &stdout2
			cmd2.Stderr = &stderr2
			if err2 := cmd2.Run(); err2 != nil {
				return "", fmt.Errorf("wongdb: jj %s: %w\n%s", strings.Join(args, " "), err2, stderr2.String())
			}
			return strings.TrimSpace(stdout2.String()), nil
		}
		return "", fmt.Errorf("wongdb: jj %s: %w\n%s", strings.Join(args, " "), err, errMsg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Init performs the full initialization flow for wong-db storage.
// It creates a dedicated jj change off root(), sets up the .wong/ directory
// structure, creates the wong-db bookmark, sets immutability, and creates
// a merge working copy with wong-db as a parent.
func (db *WongDB) Init(ctx context.Context) error {
	// Detect jj repo
	jjDir := filepath.Join(db.repoRoot, ".jj")
	if _, err := os.Stat(jjDir); os.IsNotExist(err) {
		return fmt.Errorf("wongdb: not a jj repository (no .jj/ directory in %s)", db.repoRoot)
	}

	// Check if already initialized
	if db.IsInitialized(ctx) {
		return nil
	}

	// Check if the current working copy has any content (non-empty tree).
	// If it does, we need to preserve it. If empty (fresh repo), we can
	// just create the merge working copy from scratch.
	hasContent := false
	existingFiles, _ := db.runJJ(ctx, "file", "list", "-r", "@")
	if existingFiles != "" && strings.TrimSpace(existingFiles) != "" {
		hasContent = true
	}

	// Save the parent(s) of the current working copy for later restoration.
	// We use the parent commit IDs, not the working copy's own change ID,
	// because jj may abandon empty working copies when we run `jj new`.
	var savedParents string
	if hasContent {
		// If there's content in @, commit it first so we can reference it
		if _, err := db.runJJ(ctx, "describe", "-m", "wong-init: snapshot before init"); err != nil {
			return fmt.Errorf("wongdb: failed to snapshot current state: %w", err)
		}
		// Save the current change's commit ID (more stable than change ID for non-empty changes)
		savedParents, _ = db.runJJ(ctx, "log", "-r", "@", "--no-graph", "-T", `commit_id`)
	}

	// Create wong-db change off root()
	if _, err := db.runJJ(ctx, "new", "root()", "-m", "wong-db: issue tracker storage"); err != nil {
		return fmt.Errorf("wongdb: failed to create wong-db change: %w", err)
	}

	// Create .wong/ directory structure in the working copy
	issuesDir := filepath.Join(db.repoRoot, wongIssuesDir)
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return fmt.Errorf("wongdb: failed to create issues directory: %w", err)
	}

	// Write config.json
	cfg := Config{
		Prefix:      "",
		HistoryMode: "squash",
	}
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("wongdb: failed to marshal config: %w", err)
	}
	cfgPath := filepath.Join(db.repoRoot, wongDir, "config.json")
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		return fmt.Errorf("wongdb: failed to write config: %w", err)
	}

	// Write metadata.json
	meta := Metadata{
		Version:   1,
		Backend:   "jj-native",
		CreatedAt: time.Now(),
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("wongdb: failed to marshal metadata: %w", err)
	}
	metaPath := filepath.Join(db.repoRoot, wongDir, "metadata.json")
	if err := os.WriteFile(metaPath, metaData, 0o644); err != nil {
		return fmt.Errorf("wongdb: failed to write metadata: %w", err)
	}

	// Snapshot by re-describing (jj auto-snapshots working copy changes)
	if _, err := db.runJJ(ctx, "describe", "-m", "wong-db: issue tracker storage"); err != nil {
		return fmt.Errorf("wongdb: failed to snapshot wong-db change: %w", err)
	}

	// Create bookmark
	if _, err := db.runJJ(ctx, "bookmark", "create", wongDBBookmark); err != nil {
		return fmt.Errorf("wongdb: failed to create bookmark: %w", err)
	}

	// Set immutability for wong-db
	if _, err := db.runJJ(ctx, "config", "set", "--repo",
		`revset-aliases."immutable_heads()"`, wongDBBookmark); err != nil {
		return fmt.Errorf("wongdb: failed to set immutability: %w", err)
	}

	// Create the merge working copy.
	// If the repo had existing content, merge that commit with wong-db.
	// If the repo was fresh (empty), just create a child of wong-db.
	// Note: git backend doesn't support merge commits with root() as a parent,
	// so fresh repos use a single-parent WC off wong-db.
	if hasContent && savedParents != "" {
		// Create merge: new working copy with both the previous commit and wong-db as parents
		if _, err := db.runJJ(ctx, "new", savedParents, wongDBBookmark); err != nil {
			return fmt.Errorf("wongdb: failed to create merge change: %w", err)
		}
	} else {
		// Fresh repo: create working copy as child of wong-db only.
		// When the user creates their first real commit, they can later
		// restructure with `jj new <commit> wong-db`.
		if _, err := db.runJJ(ctx, "new", wongDBBookmark); err != nil {
			return fmt.Errorf("wongdb: failed to create working copy change: %w", err)
		}
	}

	return nil
}

// IsInitialized checks if the wong-db bookmark exists in the repository.
func (db *WongDB) IsInitialized(ctx context.Context) bool {
	output, err := db.runJJ(ctx, "bookmark", "list")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), wongDBBookmark) {
			return true
		}
	}
	return false
}

// canonicalRepoPath returns the path to the canonical .jj/repo directory.
// For workspaces created with `jj workspace add`, the .jj/repo file is a text
// file containing the path to the main repo's .jj/repo directory. For the main
// workspace, .jj/repo is a directory itself.
func (db *WongDB) canonicalRepoPath() string {
	repoPath := filepath.Join(db.repoRoot, ".jj", "repo")
	info, err := os.Stat(repoPath)
	if err != nil {
		return repoPath
	}
	if !info.IsDir() {
		// Workspace: .jj/repo is a text file containing the canonical path
		data, err := os.ReadFile(repoPath)
		if err != nil {
			return repoPath
		}
		return strings.TrimSpace(string(data))
	}
	return repoPath
}

// syncLockPath returns the path to the file lock used to serialize Sync operations
// across multiple jj workspaces. The lock lives in the canonical .jj/repo/ directory
// so all workspaces contend on the same lock.
func (db *WongDB) syncLockPath() string {
	return filepath.Join(db.canonicalRepoPath(), "wong-sync.lock")
}

// restoreWongFiles writes saved .wong/ file contents back to disk.
func (db *WongDB) restoreWongFiles(files map[string][]byte) error {
	for rel, data := range files {
		fullPath := filepath.Join(db.repoRoot, rel)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Sync atomically squashes .wong/ changes from the working copy into the wong-db change.
// This is idempotent - it is a no-op if there are no .wong/ changes.
//
// For multi-workspace safety, Sync acquires an exclusive file lock so that only
// one workspace squashes into wong-db at a time. This prevents bookmark conflicts
// that occur when concurrent squash operations create divergent wong-db revisions.
//
// Important: jj workspace update-stale may overwrite on-disk files without
// snapshotting pending changes first. To prevent data loss, Sync saves the
// .wong/ file contents before update-stale and restores them afterward.
func (db *WongDB) Sync(ctx context.Context) error {
	// Update stale working copy (needed when another workspace modified the repo)
	db.runJJ(ctx, "workspace", "update-stale")

	// Restore only dirty files (files written by this instance) that update-stale
	// may have overwritten. This preserves our pending changes while accepting
	// other workspaces' changes to wong-db.
	if len(db.dirtyFiles) > 0 {
		db.restoreWongFiles(db.dirtyFiles)
	}

	// Acquire exclusive file lock to serialize sync across workspaces
	lockPath := db.syncLockPath()
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("wongdb: failed to open sync lock %s: %w", lockPath, err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("wongdb: failed to acquire sync lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	// After acquiring lock, update stale again (the lock holder before us
	// may have modified wong-db, making our working copy stale again)
	db.runJJ(ctx, "workspace", "update-stale")
	if len(db.dirtyFiles) > 0 {
		db.restoreWongFiles(db.dirtyFiles)
	}

	_, err = db.runJJ(ctx, "squash", "--into", wongDBBookmark, wongDir+"/",
		"-u", "--config", `revset-aliases."immutable_heads()"="none()"`)
	if err != nil {
		// Tolerate errors from no changes to squash
		if strings.Contains(err.Error(), "Nothing changed") || strings.Contains(err.Error(), "no changes") {
			return nil
		}
		return fmt.Errorf("wongdb: sync failed: %w", err)
	}

	// Clear dirty files after successful sync
	db.dirtyFiles = nil
	return nil
}

// ReadIssue reads a single issue's raw JSON bytes from the wong-db change.
func (db *WongDB) ReadIssue(ctx context.Context, id string) ([]byte, error) {
	issuePath := filepath.Join(wongIssuesDir, id+".json")
	output, err := db.runJJ(ctx, "file", "show", "-r", wongDBBookmark, issuePath)
	if err != nil {
		return nil, fmt.Errorf("wongdb: failed to read issue %s: %w", id, err)
	}
	return []byte(output), nil
}

// ListIssueIDs returns the IDs of all issues stored in wong-db.
// It lists files in .wong/issues/ and extracts IDs from filenames.
func (db *WongDB) ListIssueIDs(ctx context.Context) ([]string, error) {
	output, err := db.runJJ(ctx, "file", "list", "-r", wongDBBookmark, wongIssuesDir+"/")
	if err != nil {
		// No issues directory or empty - return empty list
		return nil, nil
	}

	var ids []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines are like ".wong/issues/abc123.json" - extract the ID
		base := filepath.Base(line)
		if strings.HasSuffix(base, ".json") {
			id := strings.TrimSuffix(base, ".json")
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// WriteIssue writes an issue's raw JSON data to the working copy filesystem.
// The caller should call Sync() afterward to persist the change to wong-db.
func (db *WongDB) WriteIssue(ctx context.Context, id string, data []byte) error {
	issuesDir := filepath.Join(db.repoRoot, wongIssuesDir)
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		return fmt.Errorf("wongdb: failed to create issues directory: %w", err)
	}

	issuePath := filepath.Join(issuesDir, id+".json")
	if err := os.WriteFile(issuePath, data, 0o644); err != nil {
		return fmt.Errorf("wongdb: failed to write issue %s: %w", id, err)
	}

	// Track this file as dirty so it survives update-stale
	relPath := filepath.Join(wongIssuesDir, id+".json")
	if db.dirtyFiles == nil {
		db.dirtyFiles = make(map[string][]byte)
	}
	db.dirtyFiles[relPath] = append([]byte(nil), data...) // copy
	return nil
}

// DeleteIssue removes an issue file from the working copy filesystem.
// The caller should call Sync() afterward to persist the deletion to wong-db.
func (db *WongDB) DeleteIssue(ctx context.Context, id string) error {
	issuePath := filepath.Join(db.repoRoot, wongIssuesDir, id+".json")
	if err := os.Remove(issuePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("wongdb: issue %s not found: %w", id, err)
		}
		return fmt.Errorf("wongdb: failed to delete issue %s: %w", id, err)
	}
	return nil
}

// ReadConfig reads .wong/config.json from the wong-db change.
func (db *WongDB) ReadConfig(ctx context.Context) (*Config, error) {
	configPath := filepath.Join(wongDir, "config.json")
	output, err := db.runJJ(ctx, "file", "show", "-r", wongDBBookmark, configPath)
	if err != nil {
		return nil, fmt.Errorf("wongdb: failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(output), &cfg); err != nil {
		return nil, fmt.Errorf("wongdb: failed to parse config: %w", err)
	}
	return &cfg, nil
}

// Push pushes the wong-db bookmark to the remote so other clones can access the issue data.
func (db *WongDB) Push(ctx context.Context) error {
	// First sync any local changes
	if err := db.Sync(ctx); err != nil {
		return fmt.Errorf("wongdb: push: sync failed: %w", err)
	}

	// Try to push the wong-db bookmark. May need to track first.
	_, err := db.runJJ(ctx, "git", "push", "-b", wongDBBookmark)
	if err != nil {
		// If bookmark not tracked, try tracking it first
		if strings.Contains(err.Error(), "Refusing to create") {
			if _, trackErr := db.runJJ(ctx, "bookmark", "track",
				wongDBBookmark+"@origin"); trackErr != nil {
				// Tracking failed - might be first push, try with --allow-new
				if _, err2 := db.runJJ(ctx, "git", "push", "-b", wongDBBookmark,
					"--allow-new"); err2 != nil {
					return fmt.Errorf("wongdb: push failed: %w", err2)
				}
				return nil
			}
			// Track succeeded, retry push
			if _, err2 := db.runJJ(ctx, "git", "push", "-b", wongDBBookmark); err2 != nil {
				return fmt.Errorf("wongdb: push failed after tracking: %w", err2)
			}
			return nil
		}
		return fmt.Errorf("wongdb: push failed: %w", err)
	}
	return nil
}

// Pull fetches the wong-db bookmark from the remote and updates the local copy.
func (db *WongDB) Pull(ctx context.Context) error {
	// Fetch from remote
	if _, err := db.runJJ(ctx, "git", "fetch"); err != nil {
		return fmt.Errorf("wongdb: pull: fetch failed: %w", err)
	}

	// Check if remote wong-db exists and update local bookmark
	// jj automatically tracks remote bookmarks on fetch
	return nil
}

// EnsureMergeParent ensures the current working copy has wong-db as a parent.
// This is useful after Pull when the working copy might not have wong-db as a parent.
func (db *WongDB) EnsureMergeParent(ctx context.Context) error {
	// Update stale working copy first (needed in multi-workspace scenarios)
	db.runJJ(ctx, "workspace", "update-stale")

	// Check if wong-db is already a parent of @
	output, err := db.runJJ(ctx, "log", "-r", "parents(@) & wong-db", "--no-graph", "-T", "change_id")
	if err == nil && strings.TrimSpace(output) != "" {
		return nil // already a parent
	}

	// Add wong-db as a parent: create new merge
	// Get current parents
	parents, err := db.runJJ(ctx, "log", "-r", "parents(@)", "--no-graph", "-T", `commit_id ++ "\n"`)
	if err != nil {
		return fmt.Errorf("wongdb: failed to get current parents: %w", err)
	}

	// Build args for jj new with all current parents + wong-db
	args := []string{"new"}
	for _, p := range strings.Split(strings.TrimSpace(parents), "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			args = append(args, p)
		}
	}
	args = append(args, wongDBBookmark)

	if _, err := db.runJJ(ctx, args...); err != nil {
		return fmt.Errorf("wongdb: failed to add wong-db as parent: %w", err)
	}
	return nil
}
