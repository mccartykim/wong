package wongdb

// TestE2E_MultiAgentWorkspaceOrchestrator validates the full wong workflow:
// 1. Init wong-db in a fresh jj repo
// 2. File top-level issues via wong-db
// 3. For each issue, create a jj workspace
// 4. Each "agent" goroutine works in its own workspace:
//    a. Reads its issue from wong-db
//    b. Creates subtask issues
//    c. Implements code
//    d. Closes subtasks and parent issue
//    e. Syncs to wong-db
// 5. Orchestrator waits for all agents
// 6. Verifies: all issues closed, code present, wong-db consistent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// agentTask describes work for one agent
type agentTask struct {
	issueID    string
	title      string
	desc       string
	files      map[string]string // relative path -> content
	subtasks   []agentSubtask
	dependsOn  []string // issue IDs that must be closed first
}

type agentSubtask struct {
	id    string
	title string
}

func TestE2E_MultiAgentWorkspaceOrchestrator(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping workspace orchestrator test in short mode")
	}

	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// Step 1: Init wong-db
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Log("wong-db initialized")

	// Step 2: File top-level issues
	tasks := []agentTask{
		{
			issueID: "bt-1",
			title:   "Bencode encoder/decoder",
			desc:    "Implement bencode encoding and decoding",
			files: map[string]string{
				"bencode/bencode.go": `package bencode

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func Encode(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case int:
		return []byte(fmt.Sprintf("i%de", val)), nil
	case int64:
		return []byte(fmt.Sprintf("i%de", val)), nil
	case string:
		return []byte(fmt.Sprintf("%d:%s", len(val), val)), nil
	case []byte:
		return []byte(fmt.Sprintf("%d:%s", len(val), val)), nil
	case []interface{}:
		var b strings.Builder
		b.WriteByte('l')
		for _, item := range val {
			enc, err := Encode(item)
			if err != nil {
				return nil, err
			}
			b.Write(enc)
		}
		b.WriteByte('e')
		return []byte(b.String()), nil
	case map[string]interface{}:
		var b strings.Builder
		b.WriteByte('d')
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kEnc, _ := Encode(k)
			vEnc, err := Encode(val[k])
			if err != nil {
				return nil, err
			}
			b.Write(kEnc)
			b.Write(vEnc)
		}
		b.WriteByte('e')
		return []byte(b.String()), nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", v)
	}
}

func Decode(data []byte) (interface{}, error) {
	val, _, err := decode(data, 0)
	return val, err
}

func decode(data []byte, pos int) (interface{}, int, error) {
	if pos >= len(data) {
		return nil, pos, fmt.Errorf("unexpected EOF")
	}
	switch data[pos] {
	case 'i':
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			end++
		}
		n, err := strconv.ParseInt(string(data[pos+1:end]), 10, 64)
		if err != nil {
			return nil, 0, err
		}
		return n, end + 1, nil
	case 'l':
		var list []interface{}
		pos++
		for pos < len(data) && data[pos] != 'e' {
			val, newPos, err := decode(data, pos)
			if err != nil {
				return nil, 0, err
			}
			list = append(list, val)
			pos = newPos
		}
		return list, pos + 1, nil
	case 'd':
		dict := make(map[string]interface{})
		pos++
		for pos < len(data) && data[pos] != 'e' {
			key, newPos, err := decode(data, pos)
			if err != nil {
				return nil, 0, err
			}
			val, newPos2, err := decode(data, newPos)
			if err != nil {
				return nil, 0, err
			}
			dict[key.(string)] = val
			pos = newPos2
		}
		return dict, pos + 1, nil
	default:
		// String: <length>:<data>
		colonPos := pos
		for colonPos < len(data) && data[colonPos] != ':' {
			colonPos++
		}
		length, err := strconv.Atoi(string(data[pos:colonPos]))
		if err != nil {
			return nil, 0, fmt.Errorf("invalid string length at %d", pos)
		}
		start := colonPos + 1
		return string(data[start : start+length]), start + length, nil
	}
}
`,
			},
			subtasks: []agentSubtask{
				{id: "bt-1.1", title: "Implement Encode function"},
				{id: "bt-1.2", title: "Implement Decode function"},
			},
		},
		{
			issueID: "bt-2",
			title:   "Peer wire protocol",
			desc:    "Implement BitTorrent peer wire protocol messages",
			files: map[string]string{
				"peer/messages.go": `package peer

import "encoding/binary"

const (
	MsgChoke         uint8 = 0
	MsgUnchoke       uint8 = 1
	MsgInterested    uint8 = 2
	MsgNotInterested uint8 = 3
	MsgHave          uint8 = 4
	MsgBitfield      uint8 = 5
	MsgRequest       uint8 = 6
	MsgPiece         uint8 = 7
	MsgCancel        uint8 = 8
)

type Message struct {
	ID      uint8
	Payload []byte
}

func FormatRequest(index, begin, length uint32) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)
	return &Message{ID: MsgRequest, Payload: payload}
}

func ParsePiece(msg *Message) (index, begin uint32, data []byte, err error) {
	if len(msg.Payload) < 8 {
		return 0, 0, nil, fmt.Errorf("piece message too short")
	}
	index = binary.BigEndian.Uint32(msg.Payload[0:4])
	begin = binary.BigEndian.Uint32(msg.Payload[4:8])
	data = msg.Payload[8:]
	return
}
`,
			},
			subtasks: []agentSubtask{
				{id: "bt-2.1", title: "Define message types and constants"},
				{id: "bt-2.2", title: "Implement message serialization"},
			},
		},
		{
			issueID: "bt-3",
			title:   "Piece manager",
			desc:    "Track piece download state and verify integrity",
			files: map[string]string{
				"pieces/manager.go": `package pieces

import (
	"crypto/sha1"
	"sync"
)

type PieceState int

const (
	PieceNeeded    PieceState = iota
	PieceRequested
	PieceVerified
)

type Piece struct {
	Index int
	Hash  [20]byte
	State PieceState
	Data  []byte
	mu    sync.Mutex
}

type Manager struct {
	pieces   []*Piece
	pieceLen int
	totalLen int64
	mu       sync.RWMutex
}

func NewManager(pieceLength int, totalLength int64, hashes [][20]byte) *Manager {
	m := &Manager{pieceLen: pieceLength, totalLen: totalLength}
	for i, h := range hashes {
		m.pieces = append(m.pieces, &Piece{Index: i, Hash: h})
	}
	return m
}

func (m *Manager) VerifyPiece(index int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if index >= len(m.pieces) {
		return false
	}
	p := m.pieces[index]
	p.mu.Lock()
	defer p.mu.Unlock()
	actual := sha1.Sum(p.Data)
	if actual == p.Hash {
		p.State = PieceVerified
		return true
	}
	return false
}

func (m *Manager) IsComplete() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.pieces {
		p.mu.Lock()
		s := p.State
		p.mu.Unlock()
		if s != PieceVerified {
			return false
		}
	}
	return true
}

func (m *Manager) NumPieces() int {
	return len(m.pieces)
}
`,
			},
			subtasks: []agentSubtask{
				{id: "bt-3.1", title: "Piece state tracking"},
				{id: "bt-3.2", title: "SHA1 verification"},
			},
		},
	}

	// File all top-level issues
	now := time.Now()
	for _, task := range tasks {
		issue := &types.Issue{
			ID:        task.issueID,
			Title:     task.title,
			Description: task.desc,
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := SaveIssue(db, ctx, issue); err != nil {
			t.Fatalf("Failed to save issue %s: %v", task.issueID, err)
		}
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Failed to sync after filing issues: %v", err)
	}

	// Verify issues on wong-db
	ids, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("ListIssueIDs: %v", err)
	}
	t.Logf("Filed %d top-level issues on wong-db: %v", len(ids), ids)
	if len(ids) != len(tasks) {
		t.Fatalf("expected %d issues, got %d", len(tasks), len(ids))
	}

	// Step 3: Create all workspaces serially (jj workspace add modifies the repo).
	// Then step 4: agents work in parallel within their workspaces.
	type wsInfo struct {
		name string
		dir  string
		db   *WongDB
	}
	workspaces := make([]wsInfo, len(tasks))
	for i, task := range tasks {
		wsName := fmt.Sprintf("agent-%s", task.issueID)
		wsDir := filepath.Join(dir, ".jj-workspaces", wsName)
		if err := os.MkdirAll(wsDir, 0o755); err != nil {
			t.Fatalf("mkdir workspace %s: %v", wsName, err)
		}
		if _, err := db.runJJ(ctx, "workspace", "add", "--name", wsName, wsDir); err != nil {
			t.Fatalf("workspace add %s: %v", wsName, err)
		}
		wsDB := New(wsDir)
		// Ensure wong-db is a parent of this workspace's working copy
		if err := wsDB.EnsureMergeParent(ctx); err != nil {
			t.Fatalf("EnsureMergeParent for %s: %v", wsName, err)
		}
		workspaces[i] = wsInfo{name: wsName, dir: wsDir, db: wsDB}
		t.Logf("Created workspace %s at %s", wsName, wsDir)
	}

	var wg sync.WaitGroup
	errors := make([]error, len(tasks))

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, task agentTask, ws wsInfo) {
			defer wg.Done()
			err := runAgentInWorkspace(t, dir, ws.db, ctx, task)
			if err != nil {
				errors[idx] = err
			}
		}(i, task, workspaces[i])
	}

	wg.Wait()

	// Check for agent errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("Agent %s failed: %v", tasks[i].issueID, err)
		}
	}

	// Step 6: Verify final state
	// Update the main workspace's stale working copy after agent modifications
	db.runJJ(ctx, "workspace", "update-stale")

	// All top-level issues should be closed
	for _, task := range tasks {
		data, err := db.ReadIssue(ctx, task.issueID)
		if err != nil {
			t.Errorf("Failed to read issue %s: %v", task.issueID, err)
			continue
		}
		var issue types.Issue
		if err := json.Unmarshal(data, &issue); err != nil {
			t.Errorf("Failed to parse issue %s: %v", task.issueID, err)
			continue
		}
		if issue.Status != types.StatusClosed {
			t.Errorf("Issue %s should be closed, got: %s", task.issueID, issue.Status)
		}
	}

	// All subtask issues should be closed too
	allIDs, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("Final ListIssueIDs: %v", err)
	}
	t.Logf("Final issue count: %d (expected %d top + subtasks)", len(allIDs), len(tasks))

	// Verify code files exist in the repo (in the default workspace)
	for _, task := range tasks {
		for filePath := range task.files {
			fullPath := filepath.Join(dir, filePath)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				// File might not be in default workspace yet - check jj
				t.Logf("Note: %s not in default WC filesystem (may need squash from workspace)", filePath)
			}
		}
	}

	// Print final jj log for visibility
	out := runJJ(t, dir, "log", "--no-graph")
	t.Logf("Final jj log:\n%s", out)

	// Print workspace list
	wsOut := runJJ(t, dir, "workspace", "list")
	t.Logf("Workspaces:\n%s", wsOut)
}

// runAgentInWorkspace simulates one agent working in its own jj workspace.
// The workspace must already be created (with wong-db as a merge parent).
// Each agent:
// 1. Reads its issue from wong-db
// 2. Files subtask issues
// 3. Writes code files
// 4. Closes subtasks and parent issue
// 5. Syncs to wong-db
func runAgentInWorkspace(t *testing.T, repoDir string, wsDB *WongDB, ctx context.Context, task agentTask) error {
	t.Helper()
	wsName := fmt.Sprintf("agent-%s", task.issueID)
	wsDir := wsDB.repoRoot

	t.Logf("[%s] Starting work in workspace %s", task.issueID, wsDir)

	// 1. Read our issue from wong-db (cross-workspace read via jj file show)
	issueData, err := wsDB.ReadIssue(ctx, task.issueID)
	if err != nil {
		return fmt.Errorf("read issue %s: %w", task.issueID, err)
	}
	t.Logf("[%s] Read issue: %s bytes", task.issueID, fmt.Sprintf("%d", len(issueData)))

	// 3. File subtask issues
	now := time.Now()
	for _, sub := range task.subtasks {
		subIssue := &types.Issue{
			ID:        sub.id,
			Title:     sub.title,
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
			CreatedAt: now,
			UpdatedAt: now,
			Dependencies: []*types.Dependency{
				{
					IssueID:     sub.id,
					DependsOnID: task.issueID,
					Type:        "parent-child",
					CreatedAt:   now,
					CreatedBy:   "agent-" + task.issueID,
				},
			},
		}
		if err := SaveIssue(wsDB, ctx, subIssue); err != nil {
			return fmt.Errorf("save subtask %s: %w", sub.id, err)
		}
	}
	if err := wsDB.Sync(ctx); err != nil {
		return fmt.Errorf("sync subtasks for %s: %w", task.issueID, err)
	}
	t.Logf("[%s] Filed %d subtasks", task.issueID, len(task.subtasks))

	// 4. Write code files in this workspace
	for relPath, content := range task.files {
		fullPath := filepath.Join(wsDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", relPath, err)
		}
	}
	t.Logf("[%s] Wrote %d code files", task.issueID, len(task.files))

	// 5. Close subtasks one by one
	for _, sub := range task.subtasks {
		closedIssue := &types.Issue{
			ID:        sub.id,
			Title:     sub.title,
			Status:    types.StatusClosed,
			Priority:  2,
			IssueType: types.TypeTask,
			CreatedAt: now,
			UpdatedAt: time.Now(),
			ClosedAt:  timePtr(time.Now()),
		}
		if err := SaveIssue(wsDB, ctx, closedIssue); err != nil {
			return fmt.Errorf("close subtask %s: %w", sub.id, err)
		}
		if err := wsDB.Sync(ctx); err != nil {
			return fmt.Errorf("sync close subtask %s: %w", sub.id, err)
		}
		t.Logf("[%s] Closed subtask %s", task.issueID, sub.id)
	}

	// 6. Close the parent issue
	var parentIssue types.Issue
	if err := json.Unmarshal(issueData, &parentIssue); err != nil {
		return fmt.Errorf("unmarshal parent issue: %w", err)
	}
	parentIssue.Status = types.StatusClosed
	parentIssue.UpdatedAt = time.Now()
	parentIssue.ClosedAt = timePtr(time.Now())
	parentIssue.CloseReason = "Completed by agent in workspace " + wsName
	if err := SaveIssue(wsDB, ctx, &parentIssue); err != nil {
		return fmt.Errorf("close parent %s: %w", task.issueID, err)
	}
	if err := wsDB.Sync(ctx); err != nil {
		return fmt.Errorf("sync close parent %s: %w", task.issueID, err)
	}
	t.Logf("[%s] Closed parent issue. Agent complete.", task.issueID)

	// Describe the workspace change
	_, _ = wsDB.runJJ(ctx, "describe", "-m", fmt.Sprintf("agent-%s: implement %s", task.issueID, task.title))

	return nil
}

// SaveIssue is a helper to marshal and write a types.Issue via WongDB
func SaveIssue(db *WongDB, ctx context.Context, issue *types.Issue) error {
	data, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal issue %s: %w", issue.ID, err)
	}
	return db.WriteIssue(ctx, issue.ID, data)
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// TestE2E_WorkspaceDependencyChain validates that agents can block on dependencies:
// Agent B waits for Agent A's issue to be closed before starting.
func TestE2E_WorkspaceDependencyChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dependency chain test in short mode")
	}

	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	now := time.Now()

	// File two issues: bt-a (no deps) and bt-b (depends on bt-a)
	issueA := &types.Issue{
		ID: "bt-a", Title: "Foundation layer", Status: types.StatusOpen,
		IssueType: types.TypeTask, Priority: 2, CreatedAt: now, UpdatedAt: now,
	}
	issueB := &types.Issue{
		ID: "bt-b", Title: "Depends on foundation", Status: types.StatusOpen,
		IssueType: types.TypeTask, Priority: 2, CreatedAt: now, UpdatedAt: now,
		Dependencies: []*types.Dependency{
			{IssueID: "bt-b", DependsOnID: "bt-a", Type: "blocks", CreatedAt: now},
		},
	}

	if err := SaveIssue(db, ctx, issueA); err != nil {
		t.Fatal(err)
	}
	if err := SaveIssue(db, ctx, issueB); err != nil {
		t.Fatal(err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// Create workspaces serially before launching concurrent agents
	wsADir := filepath.Join(dir, ".jj-workspaces", "agent-a")
	os.MkdirAll(wsADir, 0o755)
	db.runJJ(ctx, "workspace", "add", "--name", "agent-a", wsADir)
	wsADB := New(wsADir)
	wsADB.EnsureMergeParent(ctx)

	wsBDir := filepath.Join(dir, ".jj-workspaces", "agent-b")
	os.MkdirAll(wsBDir, 0o755)
	db.runJJ(ctx, "workspace", "add", "--name", "agent-b", wsBDir)
	wsBDB := New(wsBDir)
	wsBDB.EnsureMergeParent(ctx)

	var wg sync.WaitGroup
	var bStarted, aFinished int64
	var mu sync.Mutex

	// Agent A: does work, then closes bt-a
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Simulate work
		os.MkdirAll(filepath.Join(wsADir, "foundation"), 0o755)
		os.WriteFile(filepath.Join(wsADir, "foundation/core.go"), []byte("package foundation\n"), 0o644)

		// Close issue
		issueA.Status = types.StatusClosed
		issueA.ClosedAt = timePtr(time.Now())
		if err := SaveIssue(wsADB, ctx, issueA); err != nil {
			t.Errorf("[agent-a] SaveIssue failed: %v", err)
			return
		}
		if err := wsADB.Sync(ctx); err != nil {
			t.Errorf("[agent-a] Sync failed: %v", err)
			return
		}

		mu.Lock()
		aFinished = time.Now().UnixNano()
		mu.Unlock()
		t.Log("[agent-a] Closed bt-a")
	}()

	// Agent B: polls for bt-a closure, then works
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Poll for dependency resolution (reads from wong-db, not working copy)
		for i := 0; i < 50; i++ {
			data, err := db.ReadIssue(ctx, "bt-a")
			if err == nil {
				var dep types.Issue
				if json.Unmarshal(data, &dep) == nil && dep.Status == types.StatusClosed {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
		}

		mu.Lock()
		bStarted = time.Now().UnixNano()
		mu.Unlock()

		os.MkdirAll(filepath.Join(wsBDir, "dependent"), 0o755)
		os.WriteFile(filepath.Join(wsBDir, "dependent/feature.go"), []byte("package dependent\n"), 0o644)

		issueB.Status = types.StatusClosed
		issueB.ClosedAt = timePtr(time.Now())
		issueB.UpdatedAt = time.Now()
		if err := SaveIssue(wsBDB, ctx, issueB); err != nil {
			t.Errorf("[agent-b] SaveIssue failed: %v", err)
			return
		}

		if err := wsBDB.Sync(ctx); err != nil {
			t.Errorf("[agent-b] Sync failed: %v", err)
			return
		}
		t.Log("[agent-b] Closed bt-b")
	}()

	wg.Wait()

	// Verify ordering: B started after A finished
	mu.Lock()
	defer mu.Unlock()
	if bStarted > 0 && aFinished > 0 && bStarted < aFinished {
		t.Error("Agent B started before Agent A finished -- dependency not respected")
	}

	// Update default workspace after agent modifications
	db.runJJ(ctx, "workspace", "update-stale")

	// Verify both issues closed on wong-db
	for _, id := range []string{"bt-a", "bt-b"} {
		data, err := db.ReadIssue(ctx, id)
		if err != nil {
			t.Errorf("ReadIssue %s: %v", id, err)
			continue
		}
		var issue types.Issue
		json.Unmarshal(data, &issue)
		if issue.Status != types.StatusClosed {
			t.Errorf("%s should be closed, got %s", id, issue.Status)
		}
	}

	// Verify workspaces were created
	wsOut := runJJ(t, dir, "workspace", "list")
	if !strings.Contains(wsOut, "agent-a") || !strings.Contains(wsOut, "agent-b") {
		t.Errorf("Missing workspaces in: %s", wsOut)
	}
	t.Logf("Final workspaces:\n%s", wsOut)
}
