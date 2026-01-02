package storage

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a miniredis server and redis client for testing
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return mr, client
}

// TestRedisChannelRepository_AddChannel tests adding channels with feeds
func TestRedisChannelRepository_AddChannel(t *testing.T) {
	tests := []struct {
		name          string
		maxLimit      int
		existingChs   map[string][]string // channelID -> feedIDs
		channelToAdd  string
		feedToAdd     string
		expectError   bool
		errorContains string
	}{
		{
			name:         "add first channel with feed successfully",
			maxLimit:     5,
			existingChs:  map[string][]string{},
			channelToAdd: "123456789",
			feedToAdd:    "godot-official",
			expectError:  false,
		},
		{
			name: "add multiple feeds to same channel",
			maxLimit:     5,
			existingChs:  map[string][]string{"111": {"feed1"}},
			channelToAdd: "111",
			feedToAdd:    "feed2",
			expectError:  false,
		},
		{
			name: "reject duplicate channel-feed pair",
			maxLimit:     5,
			existingChs:  map[string][]string{"123": {"godot-official"}},
			channelToAdd: "123",
			feedToAdd:    "godot-official",
			expectError:   true,
			errorContains: "already subscribed",
		},
		{
			name: "reject when channel limit reached",
			maxLimit:     3,
			existingChs:  map[string][]string{"111": {"feed1"}, "222": {"feed2"}, "333": {"feed3"}},
			channelToAdd: "444",
			feedToAdd:    "feed4",
			expectError:   true,
			errorContains: "limit reached",
		},
		{
			name: "allow adding feed to existing channel at limit",
			maxLimit:     3,
			existingChs:  map[string][]string{"111": {"feed1"}, "222": {"feed2"}, "333": {"feed3"}},
			channelToAdd: "111",
			feedToAdd:    "feed2",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo, err := NewRedisChannelRepository(client, tt.maxLimit)
			require.NoError(t, err)

			// Seed existing channels with feeds
			for chID, feedIDs := range tt.existingChs {
				for _, feedID := range feedIDs {
					err := repo.AddChannel(chID, feedID)
					require.NoError(t, err)
				}
			}

			// Test add operation
			err = repo.AddChannel(tt.channelToAdd, tt.feedToAdd)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify channel-feed association was added
				feeds, err := repo.GetChannelFeeds(tt.channelToAdd)
				require.NoError(t, err)
				assert.Contains(t, feeds, tt.feedToAdd)
			}
		})
	}
}

// TestRedisChannelRepository_RemoveChannel tests removing channel-feed associations
func TestRedisChannelRepository_RemoveChannel(t *testing.T) {
	tests := []struct {
		name            string
		existingChs     map[string][]string
		channelToRemove string
		feedToRemove    string
		expectError     bool
		errorContains   string
		shouldCleanup   bool // Should channel be removed entirely
	}{
		{
			name:            "remove channel-feed association",
			existingChs:     map[string][]string{"111": {"feed1", "feed2"}},
			channelToRemove: "111",
			feedToRemove:    "feed1",
			expectError:     false,
			shouldCleanup:   false,
		},
		{
			name:            "remove last feed removes channel",
			existingChs:     map[string][]string{"111": {"feed1"}},
			channelToRemove: "111",
			feedToRemove:    "feed1",
			expectError:     false,
			shouldCleanup:   true,
		},
		{
			name:            "error removing non-existent association",
			existingChs:     map[string][]string{"111": {"feed1"}},
			channelToRemove: "111",
			feedToRemove:    "feed2",
			expectError:     true,
			errorContains:   "not subscribed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo, err := NewRedisChannelRepository(client, 10)
			require.NoError(t, err)

			// Seed channels
			for chID, feedIDs := range tt.existingChs {
				for _, feedID := range feedIDs {
					err := repo.AddChannel(chID, feedID)
					require.NoError(t, err)
				}
			}

			// Test remove
			err = repo.RemoveChannel(tt.channelToRemove, tt.feedToRemove)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify removal
				feeds, err := repo.GetChannelFeeds(tt.channelToRemove)
				require.NoError(t, err)
				
				if tt.shouldCleanup {
					// Channel should be gone entirely
					has, err := repo.HasChannel(tt.channelToRemove)
					require.NoError(t, err)
					assert.False(t, has)
				} else {
					// Feed should be removed but channel remains
					assert.NotContains(t, feeds, tt.feedToRemove)
				}
			}
		})
	}
}

// TestRedisChannelRepository_GetChannelFeeds tests retrieving feeds for a channel
func TestRedisChannelRepository_GetChannelFeeds(t *testing.T) {
	_, client := setupTestRedis(t)
	repo, err := NewRedisChannelRepository(client, 10)
	require.NoError(t, err)

	// Add channel with multiple feeds
	err = repo.AddChannel("ch1", "feed1")
	require.NoError(t, err)
	err = repo.AddChannel("ch1", "feed2")
	require.NoError(t, err)
	err = repo.AddChannel("ch1", "feed3")
	require.NoError(t, err)

	// Get feeds
	feeds, err := repo.GetChannelFeeds("ch1")
	require.NoError(t, err)
	assert.Len(t, feeds, 3)
	assert.Contains(t, feeds, "feed1")
	assert.Contains(t, feeds, "feed2")
	assert.Contains(t, feeds, "feed3")

	// Empty channel
	feeds, err = repo.GetChannelFeeds("nonexistent")
	require.NoError(t, err)
	assert.Len(t, feeds, 0)
}

// TestRedisChannelRepository_GetFeedChannels tests getting channels for a feed
func TestRedisChannelRepository_GetFeedChannels(t *testing.T) {
	_, client := setupTestRedis(t)
	repo, err := NewRedisChannelRepository(client, 10)
	require.NoError(t, err)

	// Add multiple channels to same feed
	err = repo.AddChannel("ch1", "godot-official")
	require.NoError(t, err)
	err = repo.AddChannel("ch2", "godot-official")
	require.NoError(t, err)
	err = repo.AddChannel("ch3", "other-feed")
	require.NoError(t, err)

	// Get channels for feed
	channels, err := repo.GetFeedChannels("godot-official")
	require.NoError(t, err)
	assert.Len(t, channels, 2)
	assert.Contains(t, channels, "ch1")
	assert.Contains(t, channels, "ch2")

	// Feed with no channels
	channels, err = repo.GetFeedChannels("nonexistent")
	require.NoError(t, err)
	assert.Len(t, channels, 0)
}

// TestRedisFeedRepository_RegisterFeed tests feed registration
func TestRedisFeedRepository_RegisterFeed(t *testing.T) {
	tests := []struct {
		name          string
		feed          Feed
		expectError   bool
		errorContains string
	}{
		{
			name: "register feed successfully",
			feed: Feed{
				ID:          "godot-official",
				URL:         "https://godotengine.org/rss.xml",
				Title:       "Godot Engine",
				Description: "Official Godot news",
				AddedAt:     time.Now(),
			},
			expectError: false,
		},
		{
			name: "register feed with schedule",
			feed: Feed{
				ID:          "gdquest",
				URL:         "https://gdquest.com/rss.xml",
				Title:       "GDQuest",
				Description: "Godot tutorials",
				AddedAt:     time.Now(),
				Schedule:    []string{"09:00", "15:00"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)
			repo := NewRedisFeedRepository(client)

			err := repo.RegisterFeed(tt.feed)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify feed was registered
				feed, err := repo.GetFeed(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.feed.ID, feed.ID)
				assert.Equal(t, tt.feed.URL, feed.URL)
				assert.Equal(t, tt.feed.Title, feed.Title)

				if len(tt.feed.Schedule) > 0 {
					assert.Equal(t, tt.feed.Schedule, feed.Schedule)
				}
			}
		})
	}
}

// TestRedisFeedRepository_DuplicateFeed tests duplicate feed registration
func TestRedisFeedRepository_DuplicateFeed(t *testing.T) {
	_, client := setupTestRedis(t)
	repo := NewRedisFeedRepository(client)

	feed := Feed{
		ID:      "godot-official",
		URL:     "https://godotengine.org/rss.xml",
		Title:   "Godot Engine",
		AddedAt: time.Now(),
	}

	// First registration should succeed
	err := repo.RegisterFeed(feed)
	require.NoError(t, err)

	// Second registration should fail
	err = repo.RegisterFeed(feed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// TestRedisFeedRepository_UnregisterFeed tests feed removal
func TestRedisFeedRepository_UnregisterFeed(t *testing.T) {
	_, client := setupTestRedis(t)
	repo := NewRedisFeedRepository(client)

	feed := Feed{
		ID:       "godot-official",
		URL:      "https://godotengine.org/rss.xml",
		Title:    "Godot Engine",
		AddedAt:  time.Now(),
		Schedule: []string{"09:00"},
	}

	// Register feed
	err := repo.RegisterFeed(feed)
	require.NoError(t, err)

	// Unregister feed
	err = repo.UnregisterFeed(feed.ID)
	assert.NoError(t, err)

	// Verify feed is gone
	has, err := repo.HasFeed(feed.ID)
	require.NoError(t, err)
	assert.False(t, has)

	// Schedule should also be gone
	schedule, err := repo.GetSchedule(feed.ID)
	require.NoError(t, err)
	assert.Len(t, schedule, 0)
}

// TestRedisFeedRepository_GetAllFeeds tests retrieving all feeds
func TestRedisFeedRepository_GetAllFeeds(t *testing.T) {
	_, client := setupTestRedis(t)
	repo := NewRedisFeedRepository(client)

	// Register multiple feeds
	feeds := []Feed{
		{ID: "feed1", URL: "http://example.com/1", Title: "Feed 1", AddedAt: time.Now()},
		{ID: "feed2", URL: "http://example.com/2", Title: "Feed 2", AddedAt: time.Now()},
		{ID: "feed3", URL: "http://example.com/3", Title: "Feed 3", AddedAt: time.Now()},
	}

	for _, feed := range feeds {
		err := repo.RegisterFeed(feed)
		require.NoError(t, err)
	}

	// Get all feeds
	allFeeds, err := repo.GetAllFeeds()
	require.NoError(t, err)
	assert.Len(t, allFeeds, 3)

	// Verify all feed IDs are present
	feedIDs := make(map[string]bool)
	for _, feed := range allFeeds {
		feedIDs[feed.ID] = true
	}
	assert.True(t, feedIDs["feed1"])
	assert.True(t, feedIDs["feed2"])
	assert.True(t, feedIDs["feed3"])
}

// TestRedisFeedRepository_Schedule tests schedule management
func TestRedisFeedRepository_Schedule(t *testing.T) {
	tests := []struct {
		name          string
		feedID        string
		times         []string
		expectError   bool
		errorContains string
	}{
		{
			name:        "set valid schedule",
			feedID:      "feed1",
			times:       []string{"09:00", "13:00", "18:00"},
			expectError: false,
		},
		{
			name:          "reject invalid time format",
			feedID:        "feed1",
			times:         []string{"9:00"},
			expectError:   true,
			errorContains: "invalid time format",
		},
		{
			name:          "reject invalid hour",
			feedID:        "feed1",
			times:         []string{"25:00"},
			expectError:   true,
			errorContains: "invalid time format",
		},
		{
			name:          "reject invalid minute",
			feedID:        "feed1",
			times:         []string{"12:60"},
			expectError:   true,
			errorContains: "invalid time format",
		},
		{
			name:        "set empty schedule",
			feedID:      "feed1",
			times:       []string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)
			repo := NewRedisFeedRepository(client)

			// Register feed first
			feed := Feed{
				ID:      tt.feedID,
				URL:     "http://example.com",
				Title:   "Test Feed",
				AddedAt: time.Now(),
			}
			err := repo.RegisterFeed(feed)
			require.NoError(t, err)

			// Set schedule
			err = repo.SetSchedule(tt.feedID, tt.times)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify schedule
				schedule, err := repo.GetSchedule(tt.feedID)
				require.NoError(t, err)
				assert.Equal(t, tt.times, schedule)
			}
		})
	}
}

// TestRedisFeedRepository_UpdateSchedule tests updating existing schedule
func TestRedisFeedRepository_UpdateSchedule(t *testing.T) {
	_, client := setupTestRedis(t)
	repo := NewRedisFeedRepository(client)

	// Register feed with initial schedule
	feed := Feed{
		ID:       "feed1",
		URL:      "http://example.com",
		Title:    "Test Feed",
		AddedAt:  time.Now(),
		Schedule: []string{"09:00", "15:00"},
	}
	err := repo.RegisterFeed(feed)
	require.NoError(t, err)

	// Verify initial schedule
	schedule, err := repo.GetSchedule("feed1")
	require.NoError(t, err)
	assert.Equal(t, []string{"09:00", "15:00"}, schedule)

	// Update schedule
	newSchedule := []string{"10:00", "14:00", "18:00"}
	err = repo.SetSchedule("feed1", newSchedule)
	require.NoError(t, err)

	// Verify updated schedule
	schedule, err = repo.GetSchedule("feed1")
	require.NoError(t, err)
	assert.Equal(t, newSchedule, schedule)
}

// TestIsValidTime tests time validation helper
func TestIsValidTime(t *testing.T) {
	tests := []struct {
		time  string
		valid bool
	}{
		{"00:00", true},
		{"12:30", true},
		{"23:59", true},
		{"09:05", true},
		{"9:00", false},   // Missing leading zero
		{"09:5", false},   // Missing leading zero
		{"24:00", false},  // Hour out of range
		{"12:60", false},  // Minute out of range
		{"1200", false},   // Missing colon
		{"12:00:00", false}, // Too many parts
		{"abc", false},    // Invalid format
	}

	for _, tt := range tests {
		t.Run(tt.time, func(t *testing.T) {
			result := isValidTime(tt.time)
			assert.Equal(t, tt.valid, result)
		})
	}
}

// TestHistoryRepository tests remain the same as they don't involve feeds

// TestRedisHistoryRepository_SaveAndGetGUID tests GUID operations
func TestRedisHistoryRepository_SaveAndGetGUID(t *testing.T) {
	tests := []struct {
		name     string
		guids    []string
		expected string
	}{
		{
			name:     "save single GUID",
			guids:    []string{"guid-123"},
			expected: "guid-123",
		},
		{
			name:     "save multiple GUIDs, last one is remembered",
			guids:    []string{"guid-1", "guid-2", "guid-3"},
			expected: "guid-3",
		},
		{
			name:     "overwrite GUID",
			guids:    []string{"old-guid", "new-guid"},
			expected: "new-guid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo := NewRedisHistoryRepository(client)

			// Save GUIDs
			for _, guid := range tt.guids {
				err := repo.SaveGUID("test-feed", guid)
				require.NoError(t, err)
			}

			// Get last GUID
			lastGUID, err := repo.GetLastGUID("test-feed")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, lastGUID)
		})
	}
}

// TestRedisHistoryRepository_HasGUID tests GUID existence checks
func TestRedisHistoryRepository_HasGUID(t *testing.T) {
	tests := []struct {
		name        string
		savedGUIDs  []string
		checkGUID   string
		expectFound bool
	}{
		{
			name:        "find existing GUID",
			savedGUIDs:  []string{"guid-1", "guid-2", "guid-3"},
			checkGUID:   "guid-2",
			expectFound: true,
		},
		{
			name:        "not find non-existent GUID",
			savedGUIDs:  []string{"guid-1", "guid-2"},
			checkGUID:   "guid-999",
			expectFound: false,
		},
		{
			name:        "find first GUID",
			savedGUIDs:  []string{"guid-1", "guid-2"},
			checkGUID:   "guid-1",
			expectFound: true,
		},
		{
			name:        "empty history",
			savedGUIDs:  []string{},
			checkGUID:   "any-guid",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo := NewRedisHistoryRepository(client)

			// Save GUIDs
			for _, guid := range tt.savedGUIDs {
				err := repo.SaveGUID("test-feed", guid)
				require.NoError(t, err)
			}

			// Check GUID
			found, err := repo.HasGUID("test-feed", tt.checkGUID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectFound, found)
		})
	}
}

// TestRedisHistoryRepository_EmptyState tests behavior with no data
func TestRedisHistoryRepository_EmptyState(t *testing.T) {
	_, client := setupTestRedis(t)

	// Should create new repository without error
	repo := NewRedisHistoryRepository(client)

	// Should have no last GUID
	lastGUID, err := repo.GetLastGUID("test-feed")
	require.NoError(t, err)
	assert.Empty(t, lastGUID)

	// Should be able to save
	err = repo.SaveGUID("test-feed", "new-guid")
	require.NoError(t, err)

	lastGUID, err = repo.GetLastGUID("test-feed")
	require.NoError(t, err)
	assert.Equal(t, "new-guid", lastGUID)
}
