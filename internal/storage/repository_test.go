package storage

import (
	"testing"

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

// TestRedisChannelRepository_AddChannel tests adding channels with table-driven approach
func TestRedisChannelRepository_AddChannel(t *testing.T) {
	tests := []struct {
		name          string
		maxLimit      int
		existingChs   []string
		channelToAdd  string
		expectError   bool
		errorContains string
	}{
		{
			name:         "add first channel successfully",
			maxLimit:     5,
			existingChs:  []string{},
			channelToAdd: "123456789",
			expectError:  false,
		},
		{
			name:         "add multiple channels within limit",
			maxLimit:     5,
			existingChs:  []string{"111", "222"},
			channelToAdd: "333",
			expectError:  false,
		},
		{
			name:          "reject duplicate channel",
			maxLimit:      5,
			existingChs:   []string{"123456789"},
			channelToAdd:  "123456789",
			expectError:   true,
			errorContains: "already exists",
		},
		{
			name:          "reject when limit reached",
			maxLimit:      3,
			existingChs:   []string{"111", "222", "333"},
			channelToAdd:  "444",
			expectError:   true,
			errorContains: "limit reached",
		},
		{
			name:         "allow adding at exact limit",
			maxLimit:     3,
			existingChs:  []string{"111", "222"},
			channelToAdd: "333",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo, err := NewRedisChannelRepository(client, tt.maxLimit)
			require.NoError(t, err)

			// Seed existing channels
			for _, ch := range tt.existingChs {
				err := repo.AddChannel(ch)
				require.NoError(t, err)
			}

			// Test add operation
			err = repo.AddChannel(tt.channelToAdd)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify channel was added
				has, err := repo.HasChannel(tt.channelToAdd)
				require.NoError(t, err)
				assert.True(t, has)
			}
		})
	}
}

// TestRedisChannelRepository_RemoveChannel tests removing channels
func TestRedisChannelRepository_RemoveChannel(t *testing.T) {
	tests := []struct {
		name            string
		existingChs     []string
		channelToRemove string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "remove existing channel",
			existingChs:     []string{"111", "222", "333"},
			channelToRemove: "222",
			expectError:     false,
		},
		{
			name:            "error removing non-existent channel",
			existingChs:     []string{"111", "222"},
			channelToRemove: "999",
			expectError:     true,
			errorContains:   "not found",
		},
		{
			name:            "remove last channel",
			existingChs:     []string{"111"},
			channelToRemove: "111",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo, err := NewRedisChannelRepository(client, 10)
			require.NoError(t, err)

			// Seed channels
			for _, ch := range tt.existingChs {
				err := repo.AddChannel(ch)
				require.NoError(t, err)
			}

			// Test remove
			err = repo.RemoveChannel(tt.channelToRemove)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify removal
				has, err := repo.HasChannel(tt.channelToRemove)
				require.NoError(t, err)
				assert.False(t, has)
			}
		})
	}
}

// TestJSONChannelRepository_GetAllChannels tests retrieving all channels
func TestRedisChannelRepository_GetAllChannels(t *testing.T) {
	tests := []struct {
		name        string
		channels    []string
		expectCount int
	}{
		{
			name:        "empty repository",
			channels:    []string{},
			expectCount: 0,
		},
		{
			name:        "single channel",
			channels:    []string{"123"},
			expectCount: 1,
		},
		{
			name:        "multiple channels",
			channels:    []string{"111", "222", "333", "444"},
			expectCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupTestRedis(t)

			repo, err := NewRedisChannelRepository(client, 10)
			require.NoError(t, err)

			// Add channels
			for _, ch := range tt.channels {
				err := repo.AddChannel(ch)
				require.NoError(t, err)
			}

			// Get all channels
			result, err := repo.GetAllChannels()
			require.NoError(t, err)
			assert.Len(t, result, tt.expectCount)

			// Verify all expected channels are present
			resultMap := make(map[string]bool)
			for _, ch := range result {
				resultMap[ch] = true
			}

			for _, expected := range tt.channels {
				assert.True(t, resultMap[expected], "expected channel %s not found", expected)
			}
		})
	}
}

// TestJSONChannelRepository_GetChannelCount tests counting channels
func TestRedisChannelRepository_GetChannelCount(t *testing.T) {
	_, client := setupTestRedis(t)

	repo, err := NewRedisChannelRepository(client, 10)
	require.NoError(t, err)

	// Initially empty
	count, err := repo.GetChannelCount()
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Add channels
	channels := []string{"111", "222", "333"}
	for i, ch := range channels {
		err := repo.AddChannel(ch)
		require.NoError(t, err)

		count, err = repo.GetChannelCount()
		require.NoError(t, err)
		assert.Equal(t, i+1, count)
	}

	// Remove a channel
	err = repo.RemoveChannel("222")
	require.NoError(t, err)

	count, err = repo.GetChannelCount()
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

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
				err := repo.SaveGUID(guid)
				require.NoError(t, err)
			}

			// Get last GUID
			lastGUID, err := repo.GetLastGUID()
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
				err := repo.SaveGUID(guid)
				require.NoError(t, err)
			}

			// Check GUID
			found, err := repo.HasGUID(tt.checkGUID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectFound, found)
		})
	}
}

// TestRedisChannelRepository_Concurrency tests thread safety
func TestRedisChannelRepository_Concurrency(t *testing.T) {
	_, client := setupTestRedis(t)

	repo, err := NewRedisChannelRepository(client, 100)
	require.NoError(t, err)

	// Add channels concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			channelID := string(rune('0' + id))
			_ = repo.AddChannel(channelID)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify no data corruption
	count, err := repo.GetChannelCount()
	require.NoError(t, err)
	assert.LessOrEqual(t, count, 10)
	assert.Greater(t, count, 0)
}

// TestJSONChannelRepository_LoadCorruptedFile tests error handling
// TestRedisHistoryRepository_EmptyState tests behavior with no data
func TestRedisHistoryRepository_EmptyState(t *testing.T) {
	_, client := setupTestRedis(t)

	// Should create new repository without error
	repo := NewRedisHistoryRepository(client)

	// Should have no last GUID
	lastGUID, err := repo.GetLastGUID()
	require.NoError(t, err)
	assert.Empty(t, lastGUID)

	// Should be able to save
	err = repo.SaveGUID("new-guid")
	require.NoError(t, err)

	lastGUID, err = repo.GetLastGUID()
	require.NoError(t, err)
	assert.Equal(t, "new-guid", lastGUID)
}
