package github

import (
	"time"
)

// PullRequest represents a GitHub PR with relevant metadata
type PullRequest struct {
	ID        int64     `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	State     string    `json:"state"`
	MergedAt  *time.Time `json:"merged_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Labels    []Label   `json:"labels"`
	Author    string    `json:"author"`
	Files     []File    `json:"files,omitempty"`
}

// Label represents a GitHub label
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// File represents a changed file in a PR
type File struct {
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Additions int   `json:"additions"`
	Deletions int   `json:"deletions"`
}

// Repository represents a GitHub repository configuration
type Repository struct {
	ID           string    `json:"id"`           // Unique identifier
	Owner        string    `json:"owner"`        // Repository owner
	Name         string    `json:"name"`         // Repository name
	TargetBranch string    `json:"target_branch"` // Branch to monitor (default: main/master)
	AddedAt      time.Time `json:"added_at"`
	LastChecked  time.Time `json:"last_checked,omitempty"`
	Schedule     []string  `json:"schedule,omitempty"` // Check times in HH:MM format (e.g., ["09:00", "13:00", "18:00"])
}

// FilterConfig defines high-value filtering criteria
type FilterConfig struct {
	LabelWhitelist []string // Labels that indicate high-value PRs
	PathExclusions []string // File patterns to exclude (e.g., "*.md", "*.txt")
	MinChanges     int      // Minimum number of changed lines
}

// DefaultFilterConfig returns sensible defaults for filtering
func DefaultFilterConfig() FilterConfig {
	return FilterConfig{
		LabelWhitelist: []string{
			// Features & Enhancements
			"feature", "enhancement", "new feature", "improvement",
			// Bug Fixes
			"bug", "bugfix", "fix", "crash",
			// Performance & Optimization
			"performance", "perf", "optimization", "speed",
			// Architecture & Core
			"architecture", "refactor", "core", "api",
			// UI/UX
			"ui", "ux", "usability", "editor",
			// Important Changes
			"major", "breaking", "security", "critical",
			// Categories (for repos that use category labels)
			"rendering", "physics", "networking", "audio", "animation",
			"scripting", "gdscript", "c#", "2d", "3d",
			// Accept any topic label (broad catch-all)
			"topic:",
		},
		PathExclusions: []string{
			".github/workflows",
			"docs/",
		},
		MinChanges: 3, // Lower threshold to catch more PRs
	}
}

// PRSummary represents a batch summary of PRs
type PRSummary struct {
	RepositoryID   string
	RepositoryName string
	PRCount        int
	Categories     map[string][]PullRequest
	GeneratedAt    time.Time
	Summary        string // Gemini-generated summary
}
