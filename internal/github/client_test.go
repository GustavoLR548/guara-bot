package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsHighValuePR(t *testing.T) {
	config := DefaultFilterConfig()
	
	tests := []struct {
		name     string
		pr       PullRequest
		expected bool
	}{
		{
			name: "high-value PR with feature label",
			pr: PullRequest{
				Number: 1,
				Title:  "Add new feature",
				Labels: []Label{
					{Name: "feature"},
				},
				Files: []File{
					{Filename: "src/main.go", Additions: 100, Deletions: 10},
				},
			},
			expected: true,
		},
		{
			name: "PR with only documentation changes",
			pr: PullRequest{
				Number: 2,
				Title:  "Update README",
				Labels: []Label{
					{Name: "feature"},
				},
				Files: []File{
					{Filename: "README.md", Additions: 5, Deletions: 2},
					{Filename: "docs/guide.md", Additions: 10, Deletions: 0},
				},
			},
			expected: false,
		},
		{
			name: "PR without high-value labels",
			pr: PullRequest{
				Number: 3,
				Title:  "Update dependencies",
				Labels: []Label{
					{Name: "dependencies"},
				},
				Files: []File{
					{Filename: "go.mod", Additions: 3, Deletions: 3},
				},
			},
			expected: false,
		},
		{
			name: "high-value PR with minimal changes",
			pr: PullRequest{
				Number: 4,
				Title:  "Fix critical bug",
				Labels: []Label{
					{Name: "bug"},
				},
				Files: []File{
					{Filename: "src/main.go", Additions: 2, Deletions: 1},
				},
			},
			expected: false, // Below MinChanges threshold
		},
		{
			name: "high-value PR with mixed files",
			pr: PullRequest{
				Number: 5,
				Title:  "Add performance improvements",
				Labels: []Label{
					{Name: "performance"},
				},
				Files: []File{
					{Filename: "README.md", Additions: 2, Deletions: 1},
					{Filename: "src/optimizer.go", Additions: 150, Deletions: 30},
				},
			},
			expected: true, // Has code changes
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsHighValuePR(tt.pr, config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCategorizePR(t *testing.T) {
	tests := []struct {
		name     string
		pr       PullRequest
		expected string
	}{
		{
			name: "UI label",
			pr: PullRequest{
				Title: "Update button styles",
				Labels: []Label{
					{Name: "ui"},
				},
			},
			expected: "UI/UX",
		},
		{
			name: "bug label",
			pr: PullRequest{
				Title: "Fix memory leak",
				Labels: []Label{
					{Name: "bug"},
				},
			},
			expected: "Bugfixes",
		},
		{
			name: "feature label",
			pr: PullRequest{
				Title: "Add new API endpoint",
				Labels: []Label{
					{Name: "feature"},
				},
			},
			expected: "Features",
		},
		{
			name: "performance label",
			pr: PullRequest{
				Title: "Optimize database queries",
				Labels: []Label{
					{Name: "performance"},
				},
			},
			expected: "Performance",
		},
		{
			name: "security label",
			pr: PullRequest{
				Title: "Fix security vulnerability",
				Labels: []Label{
					{Name: "security"},
				},
			},
			expected: "Security",
		},
		{
			name: "no labels - title based categorization",
			pr: PullRequest{
				Title: "Fix: memory leak in worker",
				Labels: []Label{},
			},
			expected: "Bugfixes",
		},
		{
			name: "no labels - feature in title",
			pr: PullRequest{
				Title: "feat: add user authentication",
				Labels: []Label{},
			},
			expected: "Features",
		},
		{
			name: "fallback to Core",
			pr: PullRequest{
				Title: "Update internal logic",
				Labels: []Label{},
			},
			expected: "Core",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CategorizePR(tt.pr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultFilterConfig(t *testing.T) {
	config := DefaultFilterConfig()
	
	assert.NotEmpty(t, config.LabelWhitelist)
	assert.NotEmpty(t, config.PathExclusions)
	assert.Greater(t, config.MinChanges, 0)
	
	// Check some expected labels
	assert.Contains(t, config.LabelWhitelist, "feature")
	assert.Contains(t, config.LabelWhitelist, "bug")
	assert.Contains(t, config.LabelWhitelist, "security")
	
	// Check some expected exclusions
	assert.Contains(t, config.PathExclusions, ".md")
	assert.Contains(t, config.PathExclusions, ".github/workflows")
}
