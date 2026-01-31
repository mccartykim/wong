package wongdb

// wong_workflow_test.go validates the full multi-agent wong workflow:
// 1. Lead agent reads a project spec, breaks it into component issues with research notes
// 2. Sub-agents in parallel jj workspaces self-prime by reading their issue from wong-db
// 3. Each agent files subtasks, writes code, closes issues - all synced via wong-db
// 4. Verification confirms all data persisted correctly across workspaces

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

// componentSpec describes one component for a sub-agent to build.
type componentSpec struct {
	issueID       string
	title         string
	description   string
	dependsOn     []string          // issue IDs this component depends on
	subtasks      []subtaskSpec     // subtasks the agent will create
	files         map[string]string // relative path -> file content
	researchNotes string            // simulated web-search notes from lead agent
}

// subtaskSpec describes a subtask created by a sub-agent.
type subtaskSpec struct {
	id    string
	title string
}

// runLeadAgent simulates an architect agent that reads the project spec,
// decomposes it into component issues with dependency edges and research notes,
// and writes them all to wong-db.
func runLeadAgent(t *testing.T, db *WongDB, ctx context.Context) []componentSpec {
	t.Helper()

	// Self-prime: read the project spec
	specData, err := db.ReadIssue(ctx, "http-spec")
	if err != nil {
		t.Fatalf("[lead] Failed to read project spec: %v", err)
	}
	var spec types.Issue
	if err := json.Unmarshal(specData, &spec); err != nil {
		t.Fatalf("[lead] Failed to parse project spec: %v", err)
	}
	t.Logf("[lead] Read project spec: %q", spec.Title)

	// Decompose into 5 components with dependency graph:
	//   config (no deps) -> models, router -> middleware, handler
	components := []componentSpec{
		{
			issueID:       "http-config",
			title:         "Config package",
			description:   "Server configuration: port, host, timeouts, environment.",
			dependsOn:     nil,
			researchNotes: "Research: use os.Getenv for env vars; stdlib suffices for stubs.",
			subtasks: []subtaskSpec{
				{id: "http-config.1", title: "Define Config struct"},
				{id: "http-config.2", title: "Implement Load function"},
			},
			files: map[string]string{
				"config/config.go": "package config\n\ntype Config struct {\n\tPort    int\n\tHost    string\n\tTimeout int\n}\n\nfunc Load() *Config {\n\treturn &Config{Port: 8080, Host: \"localhost\", Timeout: 30}\n}\n",
			},
		},
		{
			issueID:       "http-models",
			title:         "Models package",
			description:   "Data models: User, Response, Error.",
			dependsOn:     []string{"http-config"},
			researchNotes: "Research: plain structs with json tags for serialization.",
			subtasks: []subtaskSpec{
				{id: "http-models.1", title: "Define User model"},
				{id: "http-models.2", title: "Define Response and Error types"},
			},
			files: map[string]string{
				"models/models.go": "package models\n\ntype User struct {\n\tID    int    `json:\"id\"`\n\tName  string `json:\"name\"`\n\tEmail string `json:\"email\"`\n}\n\ntype Response struct {\n\tStatus int         `json:\"status\"`\n\tData   interface{} `json:\"data,omitempty\"`\n\tError  string      `json:\"error,omitempty\"`\n}\n",
			},
		},
		{
			issueID:       "http-router",
			title:         "Router package",
			description:   "HTTP request routing with path matching and method dispatch.",
			dependsOn:     []string{"http-config"},
			researchNotes: "Research: net/http.ServeMux handles basic routing.",
			subtasks: []subtaskSpec{
				{id: "http-router.1", title: "Define Router struct"},
				{id: "http-router.2", title: "Implement route registration"},
			},
			files: map[string]string{
				"router/router.go": "package router\n\nimport \"net/http\"\n\ntype Router struct {\n\tmux *http.ServeMux\n}\n\nfunc New() *Router {\n\treturn &Router{mux: http.NewServeMux()}\n}\n\nfunc (r *Router) Handle(pattern string, handler http.Handler) {\n\tr.mux.Handle(pattern, handler)\n}\n\nfunc (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {\n\tr.mux.ServeHTTP(w, req)\n}\n",
			},
		},
		{
			issueID:       "http-middleware",
			title:         "Middleware package",
			description:   "HTTP middleware: logging, recovery.",
			dependsOn:     []string{"http-router"},
			researchNotes: "Research: middleware is func(http.Handler) http.Handler pattern.",
			subtasks: []subtaskSpec{
				{id: "http-middleware.1", title: "Implement logging middleware"},
				{id: "http-middleware.2", title: "Implement recovery middleware"},
			},
			files: map[string]string{
				"middleware/middleware.go": "package middleware\n\nimport \"net/http\"\n\nfunc Logging(next http.Handler) http.Handler {\n\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n\t\tnext.ServeHTTP(w, r)\n\t})\n}\n\nfunc Recovery(next http.Handler) http.Handler {\n\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n\t\tdefer func() { recover() }()\n\t\tnext.ServeHTTP(w, r)\n\t})\n}\n",
			},
		},
		{
			issueID:       "http-handler",
			title:         "Handler package",
			description:   "HTTP request handlers: health check, user CRUD.",
			dependsOn:     []string{"http-router", "http-models"},
			researchNotes: "Research: use encoding/json.NewEncoder for response writing.",
			subtasks: []subtaskSpec{
				{id: "http-handler.1", title: "Implement health check handler"},
				{id: "http-handler.2", title: "Implement user list handler"},
			},
			files: map[string]string{
				"handler/handler.go": "package handler\n\nimport \"net/http\"\n\nfunc Health(w http.ResponseWriter, r *http.Request) {\n\tw.WriteHeader(http.StatusOK)\n\tw.Write([]byte(`{\"status\":\"ok\"}`))\n}\n\nfunc ListUsers(w http.ResponseWriter, r *http.Request) {\n\tw.WriteHeader(http.StatusOK)\n\tw.Write([]byte(`{\"users\":[]}`))\n}\n",
			},
		},
	}

	// Write each component as an issue to wong-db
	now := time.Now()
	for _, comp := range components {
		issue := &types.Issue{
			ID:          comp.issueID,
			Title:       comp.title,
			Description: comp.description,
			Notes:       comp.researchNotes,
			Status:      types.StatusOpen,
			Priority:    2,
			IssueType:   types.TypeTask,
			Assignee:    "agent-" + comp.issueID,
			CreatedAt:   now,
			UpdatedAt:   now,
			CreatedBy:   "lead-agent",
		}
		// Parent-child dependency to project spec
		issue.Dependencies = append(issue.Dependencies, &types.Dependency{
			IssueID:     comp.issueID,
			DependsOnID: "http-spec",
			Type:        "parent-child",
			CreatedAt:   now,
			CreatedBy:   "lead-agent",
		})
		// Blocking dependencies between components
		for _, depID := range comp.dependsOn {
			issue.Dependencies = append(issue.Dependencies, &types.Dependency{
				IssueID:     comp.issueID,
				DependsOnID: depID,
				Type:        "blocks",
				CreatedAt:   now,
				CreatedBy:   "lead-agent",
			})
		}
		if err := SaveIssue(db, ctx, issue); err != nil {
			t.Fatalf("[lead] Failed to save component %s: %v", comp.issueID, err)
		}
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("[lead] Sync failed: %v", err)
	}

	// Update project spec to in-progress
	spec.Status = types.StatusInProgress
	spec.UpdatedAt = time.Now()
	if err := SaveIssue(db, ctx, &spec); err != nil {
		t.Fatalf("[lead] Failed to update project spec: %v", err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("[lead] Sync update failed: %v", err)
	}

	t.Logf("[lead] Created %d component issues with dependencies", len(components))
	return components
}

// runSubAgent simulates one implementation agent working in its own jj workspace.
// It self-primes by reading its issue, reads dependencies, files subtasks,
// writes code, and closes everything via wong-db.
func runSubAgent(t *testing.T, wsDB *WongDB, ctx context.Context, comp componentSpec) error {
	t.Helper()
	wsDir := wsDB.repoRoot
	agentName := "agent-" + comp.issueID

	// === SELF-PRIMING: read own issue from wong-db ===
	issueData, err := wsDB.ReadIssue(ctx, comp.issueID)
	if err != nil {
		return fmt.Errorf("[%s] read own issue: %w", agentName, err)
	}
	var myIssue types.Issue
	if err := json.Unmarshal(issueData, &myIssue); err != nil {
		return fmt.Errorf("[%s] unmarshal own issue: %w", agentName, err)
	}
	t.Logf("[%s] Self-primed: %q (research: %d chars)", agentName, myIssue.Title, len(myIssue.Notes))

	// === READ DEPENDENCY ISSUES for context ===
	for _, dep := range myIssue.Dependencies {
		if dep.Type == "blocks" {
			depData, err := wsDB.ReadIssue(ctx, dep.DependsOnID)
			if err != nil {
				t.Logf("[%s] Warning: could not read dep %s: %v", agentName, dep.DependsOnID, err)
				continue
			}
			var depIssue types.Issue
			json.Unmarshal(depData, &depIssue)
			t.Logf("[%s] Read dependency: %s (%s)", agentName, dep.DependsOnID, depIssue.Status)
		}
	}

	// === CREATE SUBTASK ISSUES ===
	now := time.Now()
	for _, sub := range comp.subtasks {
		subIssue := &types.Issue{
			ID:        sub.id,
			Title:     sub.title,
			Status:    types.StatusOpen,
			Priority:  3,
			IssueType: types.TypeTask,
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: agentName,
			Dependencies: []*types.Dependency{{
				IssueID:     sub.id,
				DependsOnID: comp.issueID,
				Type:        "parent-child",
				CreatedAt:   now,
				CreatedBy:   agentName,
			}},
		}
		if err := SaveIssue(wsDB, ctx, subIssue); err != nil {
			return fmt.Errorf("[%s] save subtask %s: %w", agentName, sub.id, err)
		}
	}
	if err := wsDB.Sync(ctx); err != nil {
		return fmt.Errorf("[%s] sync subtasks: %w", agentName, err)
	}
	t.Logf("[%s] Filed %d subtasks", agentName, len(comp.subtasks))

	// === WRITE CODE FILES ===
	for relPath, content := range comp.files {
		fullPath := filepath.Join(wsDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("[%s] mkdir for %s: %w", agentName, relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("[%s] write %s: %w", agentName, relPath, err)
		}
	}
	t.Logf("[%s] Wrote %d code files", agentName, len(comp.files))

	// === CLOSE SUBTASKS ONE BY ONE ===
	for _, sub := range comp.subtasks {
		closedSub := &types.Issue{
			ID:          sub.id,
			Title:       sub.title,
			Status:      types.StatusClosed,
			Priority:    3,
			IssueType:   types.TypeTask,
			CreatedAt:   now,
			UpdatedAt:   time.Now(),
			ClosedAt:    timePtr(time.Now()),
			CloseReason: "Completed by " + agentName,
			CreatedBy:   agentName,
		}
		if err := SaveIssue(wsDB, ctx, closedSub); err != nil {
			return fmt.Errorf("[%s] close subtask %s: %w", agentName, sub.id, err)
		}
		if err := wsDB.Sync(ctx); err != nil {
			return fmt.Errorf("[%s] sync close subtask %s: %w", agentName, sub.id, err)
		}
		t.Logf("[%s] Closed subtask %s", agentName, sub.id)
	}

	// === CLOSE PARENT ISSUE ===
	myIssue.Status = types.StatusClosed
	myIssue.UpdatedAt = time.Now()
	myIssue.ClosedAt = timePtr(time.Now())
	myIssue.CloseReason = "All subtasks complete. Implemented by " + agentName
	if err := SaveIssue(wsDB, ctx, &myIssue); err != nil {
		return fmt.Errorf("[%s] close parent: %w", agentName, err)
	}
	if err := wsDB.Sync(ctx); err != nil {
		return fmt.Errorf("[%s] sync close parent: %w", agentName, err)
	}

	// Describe workspace with implementation note
	wsDB.runJJ(ctx, "describe", "-m",
		fmt.Sprintf("%s: implement %s", agentName, comp.title))

	t.Logf("[%s] Closed parent issue. Agent complete.", agentName)
	return nil
}

// TestE2E_WongWorkflow validates the full multi-agent wong workflow:
// lead agent decomposes -> sub-agents self-prime, file subtasks, implement, close.
func TestE2E_WongWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping wong workflow e2e test in short mode")
	}

	dir := setupJJRepo(t)
	db := newTestDB(t, dir)
	ctx := context.Background()

	// --- Phase 1: Init wong-db and create project spec ---
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	now := time.Now()
	projectSpec := &types.Issue{
		ID:          "http-spec",
		Title:       "Build HTTP Server",
		Description: "Build a simple HTTP server with router, handlers, middleware, models, and config.",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeEpic,
		Labels:      []string{"project-spec", "http-server"},
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "user",
	}
	if err := SaveIssue(db, ctx, projectSpec); err != nil {
		t.Fatalf("Failed to save project spec: %v", err)
	}
	if err := db.Sync(ctx); err != nil {
		t.Fatalf("Failed to sync project spec: %v", err)
	}
	t.Log("Phase 1: Project spec created on wong-db")

	// --- Phase 2: Lead agent decomposes project ---
	components := runLeadAgent(t, db, ctx)

	// Verify: 6 issues (1 spec + 5 components)
	ids, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("ListIssueIDs after lead agent: %v", err)
	}
	if len(ids) != 6 {
		t.Fatalf("Expected 6 issues after lead agent, got %d: %v", len(ids), ids)
	}
	t.Logf("Phase 2: Lead agent created %d component issues", len(components))

	// --- Phase 3: Create workspaces (serially), then sub-agents work (parallel) ---
	type wsInfo struct {
		name string
		dir  string
		db   *WongDB
	}
	workspaces := make([]wsInfo, len(components))
	for i, comp := range components {
		wsName := fmt.Sprintf("agent-%s", comp.issueID)
		wsDir := filepath.Join(dir, ".jj-workspaces", wsName)
		if err := os.MkdirAll(wsDir, 0o755); err != nil {
			t.Fatalf("mkdir workspace %s: %v", wsName, err)
		}
		if _, err := db.runJJ(ctx, "workspace", "add", "--name", wsName, wsDir); err != nil {
			t.Fatalf("workspace add %s: %v", wsName, err)
		}
		wsDB := New(wsDir)
		if err := wsDB.EnsureMergeParent(ctx); err != nil {
			t.Fatalf("EnsureMergeParent for %s: %v", wsName, err)
		}
		workspaces[i] = wsInfo{name: wsName, dir: wsDir, db: wsDB}
		t.Logf("Created workspace %s", wsName)
	}

	// Launch all sub-agents in parallel
	var wg sync.WaitGroup
	agentErrors := make([]error, len(components))
	for i, comp := range components {
		wg.Add(1)
		go func(idx int, comp componentSpec, ws wsInfo) {
			defer wg.Done()
			agentErrors[idx] = runSubAgent(t, ws.db, ctx, comp)
		}(i, comp, workspaces[i])
	}
	wg.Wait()

	for i, err := range agentErrors {
		if err != nil {
			t.Fatalf("Agent %s failed: %v", components[i].issueID, err)
		}
	}
	t.Log("Phase 3: All sub-agents completed")

	// --- Phase 4: Verification ---
	db.runJJ(ctx, "workspace", "update-stale")

	// 4a. All 5 component issues should be closed
	for _, comp := range components {
		data, err := db.ReadIssue(ctx, comp.issueID)
		if err != nil {
			t.Errorf("Failed to read component %s: %v", comp.issueID, err)
			continue
		}
		var issue types.Issue
		json.Unmarshal(data, &issue)
		if issue.Status != types.StatusClosed {
			t.Errorf("Component %s should be closed, got %s", comp.issueID, issue.Status)
		}
	}

	// 4b. Total issue count: 1 spec + 5 components + 10 subtasks = 16
	allIDs, err := db.ListIssueIDs(ctx)
	if err != nil {
		t.Fatalf("Final ListIssueIDs: %v", err)
	}
	expectedTotal := 1 + len(components)
	for _, comp := range components {
		expectedTotal += len(comp.subtasks)
	}
	if len(allIDs) != expectedTotal {
		t.Errorf("Expected %d total issues, got %d: %v", expectedTotal, len(allIDs), allIDs)
	}
	t.Logf("Final issue count: %d (expected %d)", len(allIDs), expectedTotal)

	// 4c. All subtasks should be closed
	for _, comp := range components {
		for _, sub := range comp.subtasks {
			data, err := db.ReadIssue(ctx, sub.id)
			if err != nil {
				t.Errorf("Failed to read subtask %s: %v", sub.id, err)
				continue
			}
			var issue types.Issue
			json.Unmarshal(data, &issue)
			if issue.Status != types.StatusClosed {
				t.Errorf("Subtask %s should be closed, got %s", sub.id, issue.Status)
			}
		}
	}

	// 4d. Code files exist in workspaces
	for i, comp := range components {
		for filePath := range comp.files {
			fullPath := filepath.Join(workspaces[i].dir, filePath)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				t.Errorf("Code file %s missing from workspace %s", filePath, workspaces[i].name)
			}
		}
	}

	// 4e. Research notes survived the full workflow
	for _, comp := range components {
		data, err := db.ReadIssue(ctx, comp.issueID)
		if err != nil {
			continue
		}
		var issue types.Issue
		json.Unmarshal(data, &issue)
		if !strings.Contains(issue.Notes, "Research:") {
			t.Errorf("Research notes missing from %s", comp.issueID)
		}
	}

	// 4f. Close reason and assignee persisted
	for _, comp := range components {
		issue, err := db.LoadIssue(ctx, comp.issueID)
		if err != nil {
			t.Errorf("LoadIssue %s: %v", comp.issueID, err)
			continue
		}
		if issue.CloseReason == "" {
			t.Errorf("Close reason empty for %s", comp.issueID)
		}
		if issue.Assignee != "agent-"+comp.issueID {
			t.Errorf("Assignee for %s: got %q, want %q", comp.issueID, issue.Assignee, "agent-"+comp.issueID)
		}
	}

	// --- Phase 5: Lead agent wraps up ---
	specData, _ := db.ReadIssue(ctx, "http-spec")
	var finalSpec types.Issue
	json.Unmarshal(specData, &finalSpec)
	finalSpec.Status = types.StatusClosed
	finalSpec.ClosedAt = timePtr(time.Now())
	finalSpec.UpdatedAt = time.Now()
	finalSpec.CloseReason = "All components implemented"
	SaveIssue(db, ctx, &finalSpec)
	db.Sync(ctx)

	// All 16 issues closed
	finalAll, _ := db.LoadAllIssues(ctx)
	closedCount := 0
	for _, iss := range finalAll {
		if iss.Status == types.StatusClosed {
			closedCount++
		}
	}
	if closedCount != expectedTotal {
		t.Errorf("Expected %d closed issues, got %d/%d", expectedTotal, closedCount, len(finalAll))
	}
	t.Logf("Phase 5: All %d issues closed", closedCount)

	// Diagnostic output
	out := runJJ(t, dir, "log", "--no-graph")
	t.Logf("Final jj log:\n%s", out)
	wsOut := runJJ(t, dir, "workspace", "list")
	t.Logf("Workspaces:\n%s", wsOut)
}
