package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/github"
	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes for GitHub integration
	repoPrefix          = "github:repos:"              // github:repos:{repoID}
	repoProcessedPrefix = "github:repos:%s:processed"  // github:repos:{repoID}:processed (SET)
	repoPendingPrefix   = "github:repos:%s:pending"    // github:repos:{repoID}:pending (LIST)
	repoChannelsPrefix  = "github:repos:%s:channels"   // github:repos:{repoID}:channels (SET)
	channelReposPrefix  = "github:channels:%s:repos"   // github:channels:{channelID}:repos (SET)
	repoLastCheckedKey  = "github:repos:%s:last_checked" // github:repos:{repoID}:last_checked
	repoScheduleKey     = "github:repos:%s:schedule"    // github:repos:{repoID}:schedule (LIST)
)

// GitHubRepository defines the interface for managing GitHub repository monitoring
type GitHubRepository interface {
	// RegisterRepository adds a new GitHub repository to monitor
	RegisterRepository(repo github.Repository) error
	// UnregisterRepository removes a repository from monitoring
	UnregisterRepository(repoID string) error
	// GetRepository returns repository details
	GetRepository(repoID string) (*github.Repository, error)
	// GetAllRepositories returns all registered repositories
	GetAllRepositories() ([]github.Repository, error)
	// HasRepository checks if a repository is registered
	HasRepository(repoID string) (bool, error)
	
	// Schedule management
	SetSchedule(repoID string, times []string) error
	GetSchedule(repoID string) ([]string, error)
	
	// AddRepoChannel associates a Discord channel with a repository
	AddRepoChannel(repoID, channelID string) error
	// RemoveRepoChannel removes channel association from repository
	RemoveRepoChannel(repoID, channelID string) error
	// GetRepoChannels returns all channels subscribed to a repository
	GetRepoChannels(repoID string) ([]string, error)
	// GetChannelRepos returns all repositories a channel is subscribed to
	GetChannelRepos(channelID string) ([]string, error)
	
	// ProcessedPRs management (deduplication)
	IsProcessed(repoID string, prID int64) (bool, error)
	MarkProcessed(repoID string, prID int64) error
	
	// Pending queue management (batching)
	AddToPendingQueue(repoID string, pr github.PullRequest) error
	GetPendingQueue(repoID string) ([]github.PullRequest, error)
	GetPendingCount(repoID string) (int, error)
	ClearPendingQueue(repoID string) error
	RemoveFromPendingQueue(repoID string, count int) error
	
	// Last checked timestamp
	UpdateLastChecked(repoID string, timestamp time.Time) error
	GetLastChecked(repoID string) (time.Time, error)
	
	// Language preferences (reuses existing news: keys)
	GetChannelLanguage(channelID string) (string, error)
	GetGuildLanguage(guildID string) (string, error)
}

// RedisGitHubRepository implements GitHubRepository using Redis
type RedisGitHubRepository struct {
	client *redis.Client
}

// NewRedisGitHubRepository creates a new Redis-based GitHub repository storage
func NewRedisGitHubRepository(client *redis.Client) *RedisGitHubRepository {
	return &RedisGitHubRepository{
		client: client,
	}
}

// RegisterRepository adds a new GitHub repository to monitor
func (r *RedisGitHubRepository) RegisterRepository(repo github.Repository) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf("%s%s", repoPrefix, repo.ID)
	
	// Store repository metadata as hash
	data := map[string]interface{}{
		"owner":         repo.Owner,
		"name":          repo.Name,
		"target_branch": repo.TargetBranch,
		"added_at":      repo.AddedAt.Format(time.RFC3339),
	}
	
	if err := r.client.HSet(ctx, key, data).Err(); err != nil {
		return fmt.Errorf("failed to register repository: %w", err)
	}
	
	log.Printf("Registered GitHub repository: %s/%s (ID: %s)", repo.Owner, repo.Name, repo.ID)
	
	// Store schedule if provided
	if len(repo.Schedule) > 0 {
		if err := r.SetSchedule(repo.ID, repo.Schedule); err != nil {
			return fmt.Errorf("failed to set schedule: %w", err)
		}
	}
	
	return nil
}

// UnregisterRepository removes a repository from monitoring
func (r *RedisGitHubRepository) UnregisterRepository(repoID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	// Delete repository metadata
	key := fmt.Sprintf("%s%s", repoPrefix, repoID)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to unregister repository: %w", err)
	}
	
	// Clean up associated data
	processedKey := fmt.Sprintf(repoProcessedPrefix, repoID)
	pendingKey := fmt.Sprintf(repoPendingPrefix, repoID)
	channelsKey := fmt.Sprintf(repoChannelsPrefix, repoID)
	lastCheckedKey := fmt.Sprintf(repoLastCheckedKey, repoID)
	scheduleKey := fmt.Sprintf(repoScheduleKey, repoID)
	
	if err := r.client.Del(ctx, processedKey, pendingKey, channelsKey, lastCheckedKey, scheduleKey).Err(); err != nil {
		log.Printf("Warning: failed to clean up repository data: %v", err)
	}
	
	log.Printf("Unregistered GitHub repository: %s", repoID)
	return nil
}

// GetRepository returns repository details
func (r *RedisGitHubRepository) GetRepository(repoID string) (*github.Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf("%s%s", repoPrefix, repoID)
	data, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	
	if len(data) == 0 {
		return nil, fmt.Errorf("repository not found: %s", repoID)
	}
	
	addedAt, _ := time.Parse(time.RFC3339, data["added_at"])
	
	return &github.Repository{
		ID:           repoID,
		Owner:        data["owner"],
		Name:         data["name"],
		TargetBranch: data["target_branch"],
		AddedAt:      addedAt,
	}, nil
}

// GetAllRepositories returns all registered repositories
func (r *RedisGitHubRepository) GetAllRepositories() ([]github.Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	// Scan for all repository keys (main hash only, not sub-keys)
	pattern := fmt.Sprintf("%s*", repoPrefix)
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	
	repos := make([]github.Repository, 0, len(keys))
	for _, key := range keys {
		// Skip sub-keys like github:repos:ID:channels, github:repos:ID:pending, etc.
		// Only process keys that match exactly github:repos:ID (no additional colons)
		if len(key) > len(repoPrefix) {
			remainder := key[len(repoPrefix):]
			if strings.Contains(remainder, ":") {
				// This is a sub-key, skip it
				continue
			}
		}
		
		// Extract repo ID from key
		repoID := key[len(repoPrefix):]
		
		repo, err := r.GetRepository(repoID)
		if err != nil {
			log.Printf("Warning: failed to get repository %s: %v", repoID, err)
			continue
		}
		
		repos = append(repos, *repo)
	}
	
	return repos, nil
}

// HasRepository checks if a repository is registered
func (r *RedisGitHubRepository) HasRepository(repoID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf("%s%s", repoPrefix, repoID)
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check repository: %w", err)
	}
	
	return exists > 0, nil
}

// AddRepoChannel associates a Discord channel with a repository
func (r *RedisGitHubRepository) AddRepoChannel(repoID, channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	repoChannelsKey := fmt.Sprintf(repoChannelsPrefix, repoID)
	channelReposKey := fmt.Sprintf(channelReposPrefix, channelID)
	
	// Use pipeline for atomic operation
	pipe := r.client.Pipeline()
	pipe.SAdd(ctx, repoChannelsKey, channelID)
	pipe.SAdd(ctx, channelReposKey, repoID)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to add channel to repository: %w", err)
	}
	
	log.Printf("Added channel %s to repository %s", channelID, repoID)
	return nil
}

// RemoveRepoChannel removes channel association from repository
func (r *RedisGitHubRepository) RemoveRepoChannel(repoID, channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	repoChannelsKey := fmt.Sprintf(repoChannelsPrefix, repoID)
	channelReposKey := fmt.Sprintf(channelReposPrefix, channelID)
	
	pipe := r.client.Pipeline()
	pipe.SRem(ctx, repoChannelsKey, channelID)
	pipe.SRem(ctx, channelReposKey, repoID)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to remove channel from repository: %w", err)
	}
	
	log.Printf("Removed channel %s from repository %s", channelID, repoID)
	return nil
}

// GetRepoChannels returns all channels subscribed to a repository
func (r *RedisGitHubRepository) GetRepoChannels(repoID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoChannelsPrefix, repoID)
	channels, err := r.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get repository channels: %w", err)
	}
	
	return channels, nil
}

// GetChannelRepos returns all repositories a channel is subscribed to
func (r *RedisGitHubRepository) GetChannelRepos(channelID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(channelReposPrefix, channelID)
	repos, err := r.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get channel repositories: %w", err)
	}
	
	return repos, nil
}

// IsProcessed checks if a PR has already been processed
func (r *RedisGitHubRepository) IsProcessed(repoID string, prID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoProcessedPrefix, repoID)
	exists, err := r.client.SIsMember(ctx, key, prID).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check processed PR: %w", err)
	}
	
	return exists, nil
}

// MarkProcessed marks a PR as processed
func (r *RedisGitHubRepository) MarkProcessed(repoID string, prID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoProcessedPrefix, repoID)
	if err := r.client.SAdd(ctx, key, prID).Err(); err != nil {
		return fmt.Errorf("failed to mark PR as processed: %w", err)
	}
	
	// Set TTL of 90 days to prevent infinite growth
	r.client.Expire(ctx, key, 90*24*time.Hour)
	
	return nil
}

// AddToPendingQueue adds a PR to the pending queue for batching
func (r *RedisGitHubRepository) AddToPendingQueue(repoID string, pr github.PullRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	// Serialize PR to JSON
	data, err := json.Marshal(pr)
	if err != nil {
		return fmt.Errorf("failed to serialize PR: %w", err)
	}
	
	key := fmt.Sprintf(repoPendingPrefix, repoID)
	if err := r.client.RPush(ctx, key, data).Err(); err != nil {
		return fmt.Errorf("failed to add PR to pending queue: %w", err)
	}
	
	log.Printf("Added PR #%d to pending queue for repository %s", pr.Number, repoID)
	return nil
}

// GetPendingQueue retrieves all pending PRs from the queue
func (r *RedisGitHubRepository) GetPendingQueue(repoID string) ([]github.PullRequest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoPendingPrefix, repoID)
	data, err := r.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending queue: %w", err)
	}
	
	prs := make([]github.PullRequest, 0, len(data))
	for _, item := range data {
		var pr github.PullRequest
		if err := json.Unmarshal([]byte(item), &pr); err != nil {
			log.Printf("Warning: failed to deserialize PR: %v", err)
			continue
		}
		prs = append(prs, pr)
	}
	
	return prs, nil
}

// GetPendingCount returns the number of PRs in the pending queue
func (r *RedisGitHubRepository) GetPendingCount(repoID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoPendingPrefix, repoID)
	count, err := r.client.LLen(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get pending count: %w", err)
	}
	
	return int(count), nil
}

// ClearPendingQueue removes all PRs from the pending queue
func (r *RedisGitHubRepository) ClearPendingQueue(repoID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoPendingPrefix, repoID)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to clear pending queue: %w", err)
	}
	
	log.Printf("Cleared pending queue for repository %s", repoID)
	return nil
}

// RemoveFromPendingQueue removes a specified number of PRs from the front of the queue
func (r *RedisGitHubRepository) RemoveFromPendingQueue(repoID string, count int) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	if count <= 0 {
		return nil
	}
	
	key := fmt.Sprintf(repoPendingPrefix, repoID)
	
	// Use LTRIM to keep only items after the count we want to remove
	// LTRIM key start stop keeps elements from start to stop (inclusive)
	// To remove first N items, we keep from N to end (-1)
	if err := r.client.LTrim(ctx, key, int64(count), -1).Err(); err != nil {
		return fmt.Errorf("failed to remove %d items from pending queue: %w", count, err)
	}
	
	log.Printf("Removed %d PRs from pending queue for repository %s", count, repoID)
	return nil
}

// UpdateLastChecked updates the last checked timestamp for a repository
func (r *RedisGitHubRepository) UpdateLastChecked(repoID string, timestamp time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoLastCheckedKey, repoID)
	if err := r.client.Set(ctx, key, timestamp.Unix(), 0).Err(); err != nil {
		return fmt.Errorf("failed to update last checked: %w", err)
	}
	
	return nil
}

// GetLastChecked retrieves the last checked timestamp for a repository
func (r *RedisGitHubRepository) GetLastChecked(repoID string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoLastCheckedKey, repoID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		// Never checked before, return zero time
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get last checked: %w", err)
	}
	
	timestamp, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	
	return time.Unix(timestamp, 0), nil
}

// SetSchedule sets check times for a repository
func (r *RedisGitHubRepository) SetSchedule(repoID string, times []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	// Validate time format (HH:MM)
	for _, t := range times {
		if !isValidTimeFormat(t) {
			return fmt.Errorf("invalid time format: %s (expected HH:MM)", t)
		}
	}
	
	key := fmt.Sprintf(repoScheduleKey, repoID)
	
	// Clear existing schedule and set new one
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	if len(times) > 0 {
		for _, t := range times {
			pipe.RPush(ctx, key, t)
		}
	}
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to set schedule: %w", err)
	}
	
	if len(times) > 0 {
		log.Printf("Set schedule for repository %s: %v", repoID, times)
	} else {
		log.Printf("Cleared schedule for repository %s", repoID)
	}
	return nil
}

// GetSchedule retrieves check times for a repository
func (r *RedisGitHubRepository) GetSchedule(repoID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf(repoScheduleKey, repoID)
	times, err := r.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}
	
	return times, nil
}

// isValidTimeFormat checks if time string is in HH:MM format
func isValidTimeFormat(timeStr string) bool {
	_, err := time.Parse("15:04", timeStr)
	return err == nil
}

// GetChannelLanguage retrieves the language preference for a channel
// Reuses the existing news:channels:{channelID}:language key
func (r *RedisGitHubRepository) GetChannelLanguage(channelID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf("news:channels:%s:language", channelID)
	language, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // No language set
	}
	if err != nil {
		return "", fmt.Errorf("failed to get channel language: %w", err)
	}
	
	return language, nil
}

// GetGuildLanguage retrieves the language preference for a guild
// Reuses the existing news:guilds:{guildID}:language key
func (r *RedisGitHubRepository) GetGuildLanguage(guildID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	key := fmt.Sprintf("news:guilds:%s:language", guildID)
	language, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // No language set
	}
	if err != nil {
		return "", fmt.Errorf("failed to get guild language: %w", err)
	}
	
	return language, nil
}
