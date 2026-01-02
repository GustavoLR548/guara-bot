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
	feedsPrefix     = "news:feeds:"           // news:feeds:{identifier}
	feedScheduleKey = "news:feeds:%s:schedule" // news:feeds:{identifier}:schedule
	channelFeedsKey = "news:channels:%s:feeds" // news:channels:{channelID}:feeds
	maxPendingItems = 5
	defaultTimeout  = 5 * time.Second
)

// ChannelRepository defines the interface for managing Discord channels
type ChannelRepository interface {
	// AddChannel adds a new channel with feed association if limit not exceeded
	AddChannel(channelID string, feedID string) error
	// RemoveChannel removes a channel and its feed association
	RemoveChannel(channelID string, feedID string) error
	// GetAllChannels returns all registered channel IDs (across all feeds)
	GetAllChannels() ([]string, error)
	// GetChannelFeeds returns all feed IDs associated with a channel
	GetChannelFeeds(channelID string) ([]string, error)
	// GetChannelCount returns the current number of unique channels
	GetChannelCount() (int, error)
	// HasChannel checks if a channel is registered for any feed
	HasChannel(channelID string) (bool, error)
	// GetFeedChannels returns all channels subscribed to a specific feed
	GetFeedChannels(feedID string) ([]string, error)
}

// Feed represents an RSS feed configuration
type Feed struct {
	ID          string
	URL         string
	Title       string
	Description string
	AddedAt     time.Time
	Schedule    []string // Array of times in "HH:MM" format
}

// FeedRepository defines the interface for managing RSS feeds
type FeedRepository interface {
	// RegisterFeed adds a new feed with the given identifier and URL
	RegisterFeed(feed Feed) error
	// UnregisterFeed removes a feed by identifier
	UnregisterFeed(feedID string) error
	// GetFeed returns feed details by identifier
	GetFeed(feedID string) (*Feed, error)
	// GetAllFeeds returns all registered feeds
	GetAllFeeds() ([]Feed, error)
	// HasFeed checks if a feed exists
	HasFeed(feedID string) (bool, error)
	// SetSchedule sets check times for a feed (e.g., ["09:00", "13:00", "18:00"])
	SetSchedule(feedID string, times []string) error
	// GetSchedule returns scheduled check times for a feed
	GetSchedule(feedID string) ([]string, error)
}

// HistoryRepository defines the interface for tracking posted articles per feed
type HistoryRepository interface {
	// GetLastGUID returns the last posted article GUID for a specific feed
	GetLastGUID(feedID string) (string, error)
	// SaveGUID saves a new article GUID for a specific feed
	SaveGUID(feedID, guid string) error
	// HasGUID checks if a GUID was already posted for a specific feed
	HasGUID(feedID, guid string) (bool, error)
	// AddToPending adds an article GUID to the pending queue for a specific feed (max 5)
	AddToPending(feedID, guid string) error
	// GetPending returns all pending article GUIDs for a specific feed
	GetPending(feedID string) ([]string, error)
	// RemoveFromPending removes a GUID from the pending queue for a specific feed
	RemoveFromPending(feedID, guid string) error
	// IsPending checks if a GUID is in the pending queue for a specific feed
	IsPending(feedID, guid string) (bool, error)
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

// AddChannel adds a new channel with feed association if limit not exceeded
func (r *RedisChannelRepository) AddChannel(channelID string, feedID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	channelFeedsKeyFormatted := fmt.Sprintf(channelFeedsKey, channelID)

	// Check if channel-feed pair already exists
	exists, err := r.client.SIsMember(ctx, channelFeedsKeyFormatted, feedID).Result()
	if err != nil {
		return fmt.Errorf("failed to check channel-feed existence: %w", err)
	}
	if exists {
		return fmt.Errorf("channel %s already subscribed to feed %s", channelID, feedID)
	}

	// Get unique channel count
	count, err := r.GetChannelCount()
	if err != nil {
		return fmt.Errorf("failed to get channel count: %w", err)
	}

	// Check if adding new channel (not just new feed to existing channel)
	hasChannel, err := r.HasChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to check channel: %w", err)
	}

	if !hasChannel && count >= r.maxLimit {
		return fmt.Errorf("channel limit reached (%d/%d)", count, r.maxLimit)
	}

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()
	
	// Add channel to global set
	pipe.SAdd(ctx, channelsKey, channelID)
	
	// Add feed to channel's feed set
	pipe.SAdd(ctx, channelFeedsKeyFormatted, feedID)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to add channel: %w", err)
	}

	return nil
}

// RemoveChannel removes a channel's association with a specific feed
func (r *RedisChannelRepository) RemoveChannel(channelID string, feedID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	channelFeedsKeyFormatted := fmt.Sprintf(channelFeedsKey, channelID)

	// Check if channel-feed pair exists
	exists, err := r.client.SIsMember(ctx, channelFeedsKeyFormatted, feedID).Result()
	if err != nil {
		return fmt.Errorf("failed to check channel-feed existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("channel %s not subscribed to feed %s", channelID, feedID)
	}

	// Remove feed from channel's feed set
	if err := r.client.SRem(ctx, channelFeedsKeyFormatted, feedID).Err(); err != nil {
		return fmt.Errorf("failed to remove feed from channel: %w", err)
	}

	// Check if channel has any remaining feeds
	feedCount, err := r.client.SCard(ctx, channelFeedsKeyFormatted).Result()
	if err != nil {
		return fmt.Errorf("failed to check remaining feeds: %w", err)
	}

	// If no more feeds, remove channel from global set and cleanup
	if feedCount == 0 {
		pipe := r.client.Pipeline()
		pipe.SRem(ctx, channelsKey, channelID)
		pipe.Del(ctx, channelFeedsKeyFormatted)
		
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("failed to cleanup channel: %w", err)
		}
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

// GetChannelFeeds returns all feed IDs associated with a channel
func (r *RedisChannelRepository) GetChannelFeeds(channelID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	channelFeedsKeyFormatted := fmt.Sprintf(channelFeedsKey, channelID)
	feeds, err := r.client.SMembers(ctx, channelFeedsKeyFormatted).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get channel feeds: %w", err)
	}

	return feeds, nil
}

// GetFeedChannels returns all channels subscribed to a specific feed
func (r *RedisChannelRepository) GetFeedChannels(feedID string) ([]string, error) {
	// Get all channels
	channels, err := r.GetAllChannels()
	if err != nil {
		return nil, err
	}

	// Filter channels that have this feed
	var feedChannels []string
	for _, channelID := range channels {
		feeds, err := r.GetChannelFeeds(channelID)
		if err != nil {
			continue
		}
		
		for _, fid := range feeds {
			if fid == feedID {
				feedChannels = append(feedChannels, channelID)
				break
			}
		}
	}

	return feedChannels, nil
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

// GetLastGUID returns the last posted article GUID for a specific feed
func (r *RedisHistoryRepository) GetLastGUID(feedID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	lastKey := fmt.Sprintf("%s:%s:last", historyPrefix, feedID)
	guid, err := r.client.Get(ctx, lastKey).Result()
	if err == redis.Nil {
		return "", nil // No last GUID yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to get last GUID: %w", err)
	}

	return guid, nil
}

// SaveGUID saves a new article GUID for a specific feed
func (r *RedisHistoryRepository) SaveGUID(feedID, guid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Use a pipeline for atomic operations
	pipe := r.client.Pipeline()
	
	// Save GUID to history set (with 90 days expiration to prevent unlimited growth)
	historyKey := fmt.Sprintf("%s:%s:%s", historyPrefix, feedID, guid)
	pipe.Set(ctx, historyKey, "1", 90*24*time.Hour)
	
	// Update last GUID for this feed
	lastKey := fmt.Sprintf("%s:%s:last", historyPrefix, feedID)
	pipe.Set(ctx, lastKey, guid, 0)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to save GUID: %w", err)
	}

	return nil
}

// HasGUID checks if a GUID was already posted for a specific feed
func (r *RedisHistoryRepository) HasGUID(feedID, guid string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	historyKey := fmt.Sprintf("%s:%s:%s", historyPrefix, feedID, guid)
	exists, err := r.client.Exists(ctx, historyKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check GUID: %w", err)
	}

	return exists > 0, nil
}

// AddToPending adds a GUID to the pending queue for a specific feed (FIFO, max 5 items)
func (r *RedisHistoryRepository) AddToPending(feedID, guid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	pendingKey := fmt.Sprintf("%s:%s:pending", historyPrefix, feedID)
	
	// Add to front of list (newest first)
	if err := r.client.LPush(ctx, pendingKey, guid).Err(); err != nil {
		return fmt.Errorf("failed to add to pending queue: %w", err)
	}

	// Trim to keep only last 5 items
	if err := r.client.LTrim(ctx, pendingKey, 0, maxPendingItems-1).Err(); err != nil {
		return fmt.Errorf("failed to trim pending queue: %w", err)
	}

	return nil
}

// GetPending returns all pending GUIDs for a specific feed (oldest to newest for processing)
func (r *RedisHistoryRepository) GetPending(feedID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	pendingKey := fmt.Sprintf("%s:%s:pending", historyPrefix, feedID)
	
	// Get all items from the list (in reverse order for oldest-first processing)
	guids, err := r.client.LRange(ctx, pendingKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending queue: %w", err)
	}

	// Reverse the slice to process oldest first
	for i, j := 0, len(guids)-1; i < j; i, j = i+1, j-1 {
		guids[i], guids[j] = guids[j], guids[i]
	}

	return guids, nil
}

// RemoveFromPending removes a GUID from the pending queue for a specific feed
func (r *RedisHistoryRepository) RemoveFromPending(feedID, guid string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	pendingKey := fmt.Sprintf("%s:%s:pending", historyPrefix, feedID)
	
	// Remove all occurrences of this GUID
	if err := r.client.LRem(ctx, pendingKey, 0, guid).Err(); err != nil {
		return fmt.Errorf("failed to remove from pending queue: %w", err)
	}

	return nil
}

// IsPending checks if a GUID is in the pending queue for a specific feed
func (r *RedisHistoryRepository) IsPending(feedID, guid string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	pendingKey := fmt.Sprintf("%s:%s:pending", historyPrefix, feedID)
	
	guids, err := r.client.LRange(ctx, pendingKey, 0, -1).Result()
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

// RedisFeedRepository implements FeedRepository using Redis
type RedisFeedRepository struct {
	client *redis.Client
}

// NewRedisFeedRepository creates a new Redis-based feed repository
func NewRedisFeedRepository(client *redis.Client) *RedisFeedRepository {
	return &RedisFeedRepository{
		client: client,
	}
}

// RegisterFeed adds a new feed with the given identifier and URL
func (r *RedisFeedRepository) RegisterFeed(feed Feed) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	feedKey := feedsPrefix + feed.ID

	// Check if feed already exists
	exists, err := r.client.Exists(ctx, feedKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check feed existence: %w", err)
	}
	if exists > 0 {
		return fmt.Errorf("feed %s already exists", feed.ID)
	}

	// Store feed as hash
	feedData := map[string]interface{}{
		"id":          feed.ID,
		"url":         feed.URL,
		"title":       feed.Title,
		"description": feed.Description,
		"added_at":    feed.AddedAt.Unix(),
	}

	if err := r.client.HSet(ctx, feedKey, feedData).Err(); err != nil {
		return fmt.Errorf("failed to register feed: %w", err)
	}

	// Set default schedule if provided
	if len(feed.Schedule) > 0 {
		if err := r.SetSchedule(feed.ID, feed.Schedule); err != nil {
			// Cleanup feed if schedule fails
			r.client.Del(ctx, feedKey)
			return fmt.Errorf("failed to set schedule: %w", err)
		}
	}

	return nil
}

// UnregisterFeed removes a feed by identifier
func (r *RedisFeedRepository) UnregisterFeed(feedID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	feedKey := feedsPrefix + feedID
	scheduleKey := fmt.Sprintf(feedScheduleKey, feedID)

	// Check if feed exists
	exists, err := r.client.Exists(ctx, feedKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check feed existence: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("feed %s not found", feedID)
	}

	// Delete feed and schedule
	pipe := r.client.Pipeline()
	pipe.Del(ctx, feedKey)
	pipe.Del(ctx, scheduleKey)
	
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to unregister feed: %w", err)
	}

	return nil
}

// GetFeed returns feed details by identifier
func (r *RedisFeedRepository) GetFeed(feedID string) (*Feed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	feedKey := feedsPrefix + feedID

	// Get feed data
	feedData, err := r.client.HGetAll(ctx, feedKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get feed: %w", err)
	}

	if len(feedData) == 0 {
		return nil, fmt.Errorf("feed %s not found", feedID)
	}

	// Parse added_at timestamp
	addedAtUnix := int64(0)
	if val, ok := feedData["added_at"]; ok {
		fmt.Sscanf(val, "%d", &addedAtUnix)
	}

	// Get schedule
	schedule, err := r.GetSchedule(feedID)
	if err != nil {
		schedule = []string{} // Default to empty if not set
	}

	feed := &Feed{
		ID:          feedData["id"],
		URL:         feedData["url"],
		Title:       feedData["title"],
		Description: feedData["description"],
		AddedAt:     time.Unix(addedAtUnix, 0),
		Schedule:    schedule,
	}

	return feed, nil
}

// GetAllFeeds returns all registered feeds
func (r *RedisFeedRepository) GetAllFeeds() ([]Feed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Scan for all feed keys
	pattern := feedsPrefix + "*"
	var cursor uint64
	var feedKeys []string

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(ctx, cursor, pattern, 10).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan feeds: %w", err)
		}

		// Filter out schedule keys
		for _, key := range keys {
			if !contains(key, ":schedule") {
				feedKeys = append(feedKeys, key)
			}
		}

		if cursor == 0 {
			break
		}
	}

	// Get each feed
	var feeds []Feed
	for _, key := range feedKeys {
		// Extract feed ID from key
		feedID := key[len(feedsPrefix):]
		
		feed, err := r.GetFeed(feedID)
		if err != nil {
			continue // Skip failed feeds
		}
		feeds = append(feeds, *feed)
	}

	return feeds, nil
}

// HasFeed checks if a feed exists
func (r *RedisFeedRepository) HasFeed(feedID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	feedKey := feedsPrefix + feedID
	exists, err := r.client.Exists(ctx, feedKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check feed: %w", err)
	}

	return exists > 0, nil
}

// SetSchedule sets check times for a feed
func (r *RedisFeedRepository) SetSchedule(feedID string, times []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	scheduleKey := fmt.Sprintf(feedScheduleKey, feedID)

	// Validate time format (HH:MM)
	for _, t := range times {
		if !isValidTime(t) {
			return fmt.Errorf("invalid time format: %s (expected HH:MM)", t)
		}
	}

	// Delete old schedule
	if err := r.client.Del(ctx, scheduleKey).Err(); err != nil {
		return fmt.Errorf("failed to clear old schedule: %w", err)
	}

	// Add new schedule times
	if len(times) > 0 {
		// Convert to interface{} slice for RPush
		timesInterface := make([]interface{}, len(times))
		for i, t := range times {
			timesInterface[i] = t
		}
		
		if err := r.client.RPush(ctx, scheduleKey, timesInterface...).Err(); err != nil {
			return fmt.Errorf("failed to set schedule: %w", err)
		}
	}

	return nil
}

// GetSchedule returns scheduled check times for a feed
func (r *RedisFeedRepository) GetSchedule(feedID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	scheduleKey := fmt.Sprintf(feedScheduleKey, feedID)
	times, err := r.client.LRange(ctx, scheduleKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	return times, nil
}

// Helper functions

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// isValidTime checks if a time string is in HH:MM format
func isValidTime(t string) bool {
	if len(t) != 5 || t[2] != ':' {
		return false
	}
	
	var hour, minute int
	_, err := fmt.Sscanf(t, "%02d:%02d", &hour, &minute)
	if err != nil {
		return false
	}
	
	return hour >= 0 && hour < 24 && minute >= 0 && minute < 60
}

