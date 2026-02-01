package wongdb

// Storage provides typed issue read/write on top of WongDB.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/steveyegge/beads/internal/types"
)

// LoadIssue reads an issue from wong-db by ID and deserializes it.
func (db *WongDB) LoadIssue(ctx context.Context, id string) (*types.Issue, error) {
	data, err := db.ReadIssue(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("wongdb: load issue %s: %w", id, err)
	}

	var issue types.Issue
	if err := json.Unmarshal(data, &issue); err != nil {
		return nil, fmt.Errorf("wongdb: unmarshal issue %s: %w", id, err)
	}
	return &issue, nil
}

// SaveIssue serializes an issue to JSON and writes it to the working copy.
// The caller should call Sync() afterward to persist the change to wong-db.
func (db *WongDB) SaveIssue(ctx context.Context, issue *types.Issue) error {
	if issue.ID == "" {
		return fmt.Errorf("wongdb: cannot save issue with empty ID")
	}

	data, err := json.MarshalIndent(issue, "", "  ")
	if err != nil {
		return fmt.Errorf("wongdb: marshal issue %s: %w", issue.ID, err)
	}

	if err := db.WriteIssue(ctx, issue.ID, data); err != nil {
		return fmt.Errorf("wongdb: save issue %s: %w", issue.ID, err)
	}
	return nil
}

// LoadAllIssues reads all issues from wong-db.
func (db *WongDB) LoadAllIssues(ctx context.Context) ([]*types.Issue, error) {
	ids, err := db.ListIssueIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("wongdb: list issues: %w", err)
	}

	var issues []*types.Issue
	for _, id := range ids {
		issue, err := db.LoadIssue(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("wongdb: load all issues: %w", err)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

// IsReady checks if an issue's blocking dependencies are all closed.
// An issue is "ready" if it has no unresolved blocking dependencies.
// Issues that are already closed are not considered ready.
func (db *WongDB) IsReady(ctx context.Context, id string) (bool, error) {
	issue, err := db.LoadIssue(ctx, id)
	if err != nil {
		return false, err
	}

	// Closed/tombstone issues are not ready (already done)
	if issue.Status == types.StatusClosed || issue.Status == types.StatusTombstone {
		return false, nil
	}

	// Check blocking dependencies
	for _, dep := range issue.Dependencies {
		if dep.Type != types.DepBlocks && dep.Type != types.DepWaitsFor && dep.Type != types.DepConditionalBlocks {
			continue // non-blocking dependency type
		}
		// dep.DependsOnID is what this issue depends on
		blocker, err := db.LoadIssue(ctx, dep.DependsOnID)
		if err != nil {
			// If we can't load the blocker, treat as still blocking
			return false, nil
		}
		if blocker.Status != types.StatusClosed {
			return false, nil
		}
	}

	return true, nil
}

// ReadyIssues returns all issues that are ready to be worked on.
// An issue is ready if it is not closed and all its blocking dependencies are closed.
func (db *WongDB) ReadyIssues(ctx context.Context) ([]*types.Issue, error) {
	allIssues, err := db.LoadAllIssues(ctx)
	if err != nil {
		return nil, err
	}

	// Build a map for quick lookups
	issueMap := make(map[string]*types.Issue)
	for _, issue := range allIssues {
		issueMap[issue.ID] = issue
	}

	var ready []*types.Issue
	for _, issue := range allIssues {
		// Skip already closed
		if issue.Status == types.StatusClosed || issue.Status == types.StatusTombstone {
			continue
		}

		// Check blocking dependencies
		blocked := false
		for _, dep := range issue.Dependencies {
			if dep.Type != types.DepBlocks && dep.Type != types.DepWaitsFor && dep.Type != types.DepConditionalBlocks {
				continue
			}
			blocker, ok := issueMap[dep.DependsOnID]
			if !ok || blocker.Status != types.StatusClosed {
				blocked = true
				break
			}
		}

		if !blocked {
			ready = append(ready, issue)
		}
	}

	return ready, nil
}

// RemoveIssue deletes an issue from the working copy and syncs the deletion to wong-db.
func (db *WongDB) RemoveIssue(ctx context.Context, id string) error {
	if err := db.DeleteIssue(ctx, id); err != nil {
		return fmt.Errorf("wongdb: remove issue %s: %w", id, err)
	}
	if err := db.Sync(ctx); err != nil {
		return fmt.Errorf("wongdb: remove issue %s sync: %w", id, err)
	}
	return nil
}
