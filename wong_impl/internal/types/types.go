package types

import "time"

// Status represents the lifecycle state of an issue.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusClosed     Status = "closed"
	StatusTombstone  Status = "tombstone"
)

// IssueType categorizes what kind of work an issue represents.
type IssueType string

const (
	TypeBug     IssueType = "bug"
	TypeFeature IssueType = "feature"
	TypeTask    IssueType = "task"
	TypeEpic    IssueType = "epic"
	TypeChore   IssueType = "chore"
)

// Dependency represents a relationship between two issues.
type Dependency struct {
	IssueID     string    `json:"issue_id"`
	DependsOnID string    `json:"depends_on_id"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
}

// Dependency type constants.
const (
	DepBlocks            = "blocks"
	DepWaitsFor          = "waits_for"
	DepConditionalBlocks = "conditional_blocks"
)

// Comment represents a comment on an issue.
type Comment struct {
	ID        int       `json:"id"`
	IssueID   string    `json:"issue_id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Issue is the core data model for wong issue tracking.
type Issue struct {
	ID               string        `json:"id"`
	Title            string        `json:"title"`
	Description      string        `json:"description"`
	Status           Status        `json:"status"`
	Priority         int           `json:"priority"`
	IssueType        IssueType     `json:"issue_type,omitempty"`
	Assignee         string        `json:"assignee,omitempty"`
	Owner            string        `json:"owner,omitempty"`
	CreatedBy        string        `json:"created_by,omitempty"`
	EstimatedMinutes *int          `json:"estimated_minutes,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	ClosedAt         *time.Time    `json:"closed_at,omitempty"`
	CloseReason      string        `json:"close_reason,omitempty"`
	Notes            string        `json:"notes,omitempty"`
	Labels           []string      `json:"labels,omitempty"`
	Comments         []*Comment    `json:"comments,omitempty"`
	Dependencies     []*Dependency `json:"dependencies,omitempty"`
}
