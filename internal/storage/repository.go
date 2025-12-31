package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes
	channelsKey     = "news:channels"
	historyPrefix   = "news:history:"
	lastGUIDKey     = "news:last_guid"
	maxLimitKey     = "news:config:max_channels"
	pendingQueueKey = "news:pending_queue"
	maxPendingItems = 5
	defaultTimeout  = 5 * time.Second
)

// ChannelRepository defines the interface for managing Discord channels
type ChannelRepository interface {
	// AddChannel adds a new channel if limit not exceeded
	AddChannel(channelID string) error
	// RemoveChannel removes a channel by ID
	RemoveChannel(channelID string) error
	// GetAllChannels returns all registered channel IDs
	GetAllChannels() ([]string, error)
	// GetChannelCount returns the current number of channels
	GetChannelCount() (int, error)
	// HasChannel checks if a channel is already registered
	HasChannel(channelID string) (bool, error)
}

// HistoryRepository defines the interface for tracking posted articles
type HistoryRepository interface {
	// GetLastGUID returns the last posted article GUID
	GetLastGUID() (string, error)
	// SaveGUID saves a new article GUID
	SaveGUID(guid string) error
	// HasGUID checks if a GUID was already posted
	HasGUID(guid string) (bool, error)
	// AddToPending adds an article GUID to the pending queue (max 5)
	AddToPending(guid string) error
	// GetPending returns all pending article GUIDs
	GetPending() ([]string, error)
	// RemoveFromPending removes a GUID from the pending queue
	RemoveFromPending(guid string) error
	// IsPending checks if a GUID is in the pending queue
	IsPending(guid string) (bool, error)
}

// RedisChannelRepository implements ChannelRepository using Redis
type RedisChannelRepository struct {
	client   *redis.Client
	maxLimit int
}

// NewRedisChannelRepository creates a new Redis-based channel repository
func NewRedisChannelRepository(client *redis.Client, maxLimit int) (*RedisChannelRepository, error) {
	repo := &RedisChannelRepository{
		client:   client,
		maxLimit: maxLimit,
	}

	// Store max limit in Redis for reference
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	
	if err := client.Set(ctx, maxLimitKey, maxLimit, 0).Err(); err != nil {
		return nil, fmt.Errorf("failed to set max limit: %w", err)
	}

	return repo, nil
}

// AddChannel adds a new channel if limit not exceeded
func (r *RedisChannelRepository) AddChannel(channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Check if channel already exists
	exists, err := r.client.SIsMember(ctx, channelsKey, channelID).Result()
	if err != nil {
		return fmt.Errorf("failed to check channel existence: %w", err)
	}
	if exists {
		return fmt.Errorf("channel %s already exists", channelID)
	}

	// Check current count
	count, err := r.client.SCard(ctx, channelsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get channel count: %w", err)
	}

	if int(count) >= r.maxLimit {
		return fmt.Errorf("channel limit reached (%d/%d)", count, r.maxLimit)
	}

	// Add channel
	if err := r.client.SAdd(ctx, channelsKey, channelID).Err(); err != nil {
		return fmt.Errorf("failed to add channel: %w", err)
	}

	return nil
}

// RemoveChannel removes a channel by ID
func (r *RedisChannelRepository) RemoveChannel(channelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Check if channel exists
	exists, err := r.client.SIsMember(ctx, channelsKey, channelID).Result()
	if err != nil {
		return fmt.Errorf("failed to check channel existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("channel %s not found", channelID)
	}

	// Remove channel
	if err := r.client.SRem(ctx, channelsKey, channelID).Err(); err != nil {
		return fmt.Errorf("failed to remove channel: %w", err)
	}

	return nil
}

// GetAllChannels returns all registered channel IDs
func (r *RedisChannelRepository) GetAllChannels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	channels, err := r.client.SMembers(ctx, channelsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get channels: %w", err)
	}

	return channels, nil
}

// GetChannelCount returns the current number of channels
func (r *RedisChannelRepository) GetChannelCount() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	count, err := r.client.SCard(ctx, channelsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get channel count: %w", err)
	}

	return int(count), nil
}

// HasChannel checks if a channel is already registered
func (r *RedisChannelRepository) HasChannel(channelID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	exists, err := r.client.SIsMember(ctx, channelsKey, channelID).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check channel: %w", err)
	}

	return exists, nil
}

// RedisHistoryRepository implements HistoryRepository using Redis
type RedisHistoryRepository struct {
	client *redis.Client
}

// NewRedisHistoryRepository creates a new Redis-based history repository
func NewRedisHistoryRepository(client *redis.Client) *RedisHistoryRepository {
	return &RedisHistoryRepository{
		client: client,
	}
}

// GetLastGUID returns the last posted article GUID
func (r *RedisHistoryRepository) GetLastGUID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	guid, err := r.client.Get(ctx, lastGUIDKey).Result()
	if err == redis.Nil {
		return "", nil // No last GUID yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to get last GUID: %w", err)
	}

	return guid, nil
}

// SaveGUID saves a new article GUID
func (r *RedisHistoryRepository) SaveGUID(guid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Use a pipeline for atomic operations
	pipe := r.client.Pipeline()
	
	// Save GUID to history set (with 90 days expiration to prevent unlimited growth)
	historyKey := historyPrefix + guid
	pipe.Set(ctx, historyKey, "1", 90*24*time.Hour)
	
	// Update last GUID
	pipe.Set(ctx, lastGUIDKey, guid, 0)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to save GUID: %w", err)
	}

	return nil
}

// HasGUID checks if a GUID was already posted
func (r *RedisHistoryRepository) HasGUID(guid string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	historyKey := historyPrefix + guid
	exists, err := r.client.Exists(ctx, historyKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check GUID: %w", err)
	}

	return exists > 0, nil
}

// AddToPending adds a GUID to the pending queue (FIFO, max 5 items)
func (r *RedisHistoryRepository) AddToPending(guid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Add to front of list (newest first)
	if err := r.client.LPush(ctx, pendingQueueKey, guid).Err(); err != nil {
		return fmt.Errorf("failed to add to pending queue: %w", err)
	}

	// Trim to keep only last 5 items
	if err := r.client.LTrim(ctx, pendingQueueKey, 0, maxPendingItems-1).Err(); err != nil {
		return fmt.Errorf("failed to trim pending queue: %w", err)
	}

	return nil
}

// GetPending returns all pending GUIDs (oldest to newest for processing)
func (r *RedisHistoryRepository) GetPending() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Get all items from the list (in reverse order for oldest-first processing)
	guids, err := r.client.LRange(ctx, pendingQueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending queue: %w", err)
	}

	// Reverse the slice to process oldest first
	for i, j := 0, len(guids)-1; i < j; i, j = i+1, j-1 {
		guids[i], guids[j] = guids[j], guids[i]
	}

	return guids, nil
}

// RemoveFromPending removes a GUID from the pending queue
func (r *RedisHistoryRepository) RemoveFromPending(guid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Remove all occurrences of this GUID
	if err := r.client.LRem(ctx, pendingQueueKey, 0, guid).Err(); err != nil {
		return fmt.Errorf("failed to remove from pending queue: %w", err)
	}

	return nil
}

// IsPending checks if a GUID is in the pending queue
func (r *RedisHistoryRepository) IsPending(guid string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	guids, err := r.client.LRange(ctx, pendingQueueKey, 0, -1).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check pending queue: %w", err)
	}

	for _, g := range guids {
		if g == guid {
			return true, nil
		}
	}

	return false, nil
}
