package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client handles GitHub API interactions
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new GitHub API client
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.github.com",
	}
}

// FetchMergedPRs fetches recently merged PRs from a repository
func (c *Client) FetchMergedPRs(ctx context.Context, owner, repo, targetBranch string, since time.Time) ([]PullRequest, error) {
	// GitHub API endpoint for pull requests
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=closed&sort=updated&direction=desc&per_page=100", 
		c.baseURL, owner, repo)
	
	log.Printf("Fetching merged PRs from %s/%s (branch: %s)", owner, repo, targetBranch)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PRs: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %d - %s", resp.StatusCode, string(body))
	}
	
	var prs []struct {
		ID     int64  `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		HTMLURL string `json:"html_url"`
		State  string `json:"state"`
		MergedAt *time.Time `json:"merged_at"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"labels"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	// Filter and convert to our model
	var result []PullRequest
	for _, pr := range prs {
		// Only include merged PRs
		if pr.MergedAt == nil {
			continue
		}
		
		// Check if merged to target branch
		if targetBranch != "" && pr.Base.Ref != targetBranch {
			continue
		}
		
		// Check if merged since the specified time
		if pr.MergedAt.Before(since) {
			continue
		}
		
		// Convert labels
		labels := make([]Label, len(pr.Labels))
		for i, l := range pr.Labels {
			labels[i] = Label{
				Name:  l.Name,
				Color: l.Color,
			}
		}
		
		result = append(result, PullRequest{
			ID:        pr.ID,
			Number:    pr.Number,
			Title:     pr.Title,
			Body:      pr.Body,
			HTMLURL:   pr.HTMLURL,
			State:     pr.State,
			MergedAt:  pr.MergedAt,
			CreatedAt: pr.CreatedAt,
			UpdatedAt: pr.UpdatedAt,
			Labels:    labels,
			Author:    pr.User.Login,
		})
	}
	
	log.Printf("Found %d merged PRs in %s/%s", len(result), owner, repo)
	return result, nil
}

// FetchPRFiles fetches the list of changed files for a specific PR
func (c *Client) FetchPRFiles(ctx context.Context, owner, repo string, prNumber int) ([]File, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files", c.baseURL, owner, repo, prNumber)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR files: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error: %d - %s", resp.StatusCode, string(body))
	}
	
	var files []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("failed to decode files: %w", err)
	}
	
	result := make([]File, len(files))
	for i, f := range files {
		result[i] = File{
			Filename:  f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
		}
	}
	
	return result, nil
}

// IsHighValuePR determines if a PR meets high-value criteria
func IsHighValuePR(pr PullRequest, config FilterConfig) bool {
	// Start detailed logging
	log.Printf("[GITHUB-CLIENT] Checking PR #%d: \"%s\"", pr.Number, pr.Title)
	
	// Log PR labels
	labelNames := make([]string, len(pr.Labels))
	for i, label := range pr.Labels {
		labelNames[i] = label.Name
	}
	if len(labelNames) > 0 {
		log.Printf("[GITHUB-CLIENT]   Labels: [%s]", strings.Join(labelNames, ", "))
	} else {
		log.Printf("[GITHUB-CLIENT]   Labels: []")
	}
	
	// Check label whitelist
	hasHighValueLabel := false
	matchedLabel := ""
	for _, prLabel := range pr.Labels {
		for _, whitelistLabel := range config.LabelWhitelist {
			// Check for exact match or prefix match (for patterns like "topic:")
			if strings.EqualFold(prLabel.Name, whitelistLabel) || 
			   strings.HasPrefix(strings.ToLower(prLabel.Name), strings.ToLower(whitelistLabel)) {
				hasHighValueLabel = true
				matchedLabel = prLabel.Name
				log.Printf("[GITHUB-CLIENT]   Matched whitelist label: %s", prLabel.Name)
				break
			}
		}
		if hasHighValueLabel {
			break
		}
	}
	
	if !hasHighValueLabel {
		log.Printf("[GITHUB-CLIENT]   No matching labels")
		log.Printf("[GITHUB-CLIENT]   ❌ PR #%d rejected (no high-value labels)", pr.Number)
		return false
	}
	
	// Check if files contain excluded paths (if files are available)
	if len(pr.Files) > 0 {
		allExcluded := true
		totalChanges := 0
		
		for _, file := range pr.Files {
			totalChanges += file.Additions + file.Deletions
			
			excluded := false
			for _, pattern := range config.PathExclusions {
				if strings.HasSuffix(file.Filename, pattern) || strings.HasPrefix(file.Filename, pattern) {
					excluded = true
					break
				}
			}
			
			if !excluded {
				allExcluded = false
			}
		}
		
		log.Printf("[GITHUB-CLIENT]   Changes: %d lines (min: %d)", totalChanges, config.MinChanges)
		
		if allExcluded {
			log.Printf("[GITHUB-CLIENT]   ❌ PR #%d rejected (only modifies excluded files)", pr.Number)
			return false
		}
		
		if totalChanges < config.MinChanges {
			log.Printf("[GITHUB-CLIENT]   ❌ PR #%d rejected (too few changes: %d < %d)", pr.Number, totalChanges, config.MinChanges)
			return false
		}
	} else {
		log.Printf("[GITHUB-CLIENT]   Changes: unknown (no file data)")
	}
	
	log.Printf("[GITHUB-CLIENT]   ✅ PR #%d accepted (label: %s)", pr.Number, matchedLabel)
	return true
}

// CategorizePR attempts to categorize a PR based on labels and title
func CategorizePR(pr PullRequest) string {
	// Check labels first
	for _, label := range pr.Labels {
		labelLower := strings.ToLower(label.Name)
		switch {
		case strings.Contains(labelLower, "ui") || strings.Contains(labelLower, "ux"):
			return "UI/UX"
		case strings.Contains(labelLower, "bug") || strings.Contains(labelLower, "fix"):
			return "Bugfixes"
		case strings.Contains(labelLower, "feature") || strings.Contains(labelLower, "enhancement"):
			return "Features"
		case strings.Contains(labelLower, "perf") || strings.Contains(labelLower, "performance"):
			return "Performance"
		case strings.Contains(labelLower, "doc"):
			return "Documentation"
		case strings.Contains(labelLower, "security"):
			return "Security"
		case strings.Contains(labelLower, "breaking"):
			return "Breaking Changes"
		}
	}
	
	// Fallback to title analysis
	titleLower := strings.ToLower(pr.Title)
	switch {
	case strings.Contains(titleLower, "ui") || strings.Contains(titleLower, "ux"):
		return "UI/UX"
	case strings.HasPrefix(titleLower, "fix") || strings.Contains(titleLower, "bug"):
		return "Bugfixes"
	case strings.HasPrefix(titleLower, "feat") || strings.HasPrefix(titleLower, "add"):
		return "Features"
	case strings.Contains(titleLower, "perf") || strings.Contains(titleLower, "optim"):
		return "Performance"
	default:
		return "Core"
	}
}
