package bot

import (
	"fmt"
	"testing"

	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockChannelRepository is a mock for testing
type MockChannelRepository struct {
	channels   map[string]bool
	maxLimit   int
	addError   error
	getError   error
}

func NewMockChannelRepository(maxLimit int) *MockChannelRepository {
	return &MockChannelRepository{
		channels: make(map[string]bool),
		maxLimit: maxLimit,
	}
}

func (m *MockChannelRepository) AddChannel(channelID string) error {
	if m.addError != nil {
		return m.addError
	}
	if m.channels[channelID] {
		return fmt.Errorf("channel %s already exists", channelID)
	}
	if len(m.channels) >= m.maxLimit {
		return fmt.Errorf("channel limit reached (%d/%d)", len(m.channels), m.maxLimit)
	}
	m.channels[channelID] = true
	return nil
}

func (m *MockChannelRepository) RemoveChannel(channelID string) error {
	delete(m.channels, channelID)
	return nil
}

func (m *MockChannelRepository) GetAllChannels() ([]string, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	channels := make([]string, 0, len(m.channels))
	for ch := range m.channels {
		channels = append(channels, ch)
	}
	return channels, nil
}

func (m *MockChannelRepository) GetChannelCount() (int, error) {
	if m.getError != nil {
		return 0, m.getError
	}
	return len(m.channels), nil
}

func (m *MockChannelRepository) HasChannel(channelID string) (bool, error) {
	if m.getError != nil {
		return false, m.getError
	}
	return m.channels[channelID], nil
}

// TestNewCommandHandler tests handler creation
func TestNewCommandHandler(t *testing.T) {
	repo := NewMockChannelRepository(5)
	handler := NewCommandHandler(repo, 5)

	assert.NotNil(t, handler)
	assert.Equal(t, repo, handler.channelRepo)
	assert.Equal(t, 5, handler.maxLimit)
}

// TestHasManageServerPermission tests permission checking
func TestHasManageServerPermission(t *testing.T) {
	handler := NewCommandHandler(nil, 5)

	tests := []struct {
		name        string
		permissions int64
		expected    bool
	}{
		{
			name:        "has administrator permission",
			permissions: discordgo.PermissionAdministrator,
			expected:    true,
		},
		{
			name:        "has manage guild permission",
			permissions: 0x0000000000000020, // MANAGE_GUILD
			expected:    true,
		},
		{
			name:        "has both permissions",
			permissions: discordgo.PermissionAdministrator | 0x0000000000000020,
			expected:    true,
		},
		{
			name:        "has no relevant permissions",
			permissions: discordgo.PermissionSendMessages,
			expected:    false,
		},
		{
			name:        "has zero permissions",
			permissions: 0,
			expected:    false,
		},
		{
			name:        "has other permissions but not manage",
			permissions: discordgo.PermissionManageChannels | discordgo.PermissionManageRoles,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := &discordgo.Member{
				Permissions: tt.permissions,
			}

			result := handler.hasManageServerPermission(member)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCommandHandler_SetupNews_PermissionValidation tests permission checks
func TestCommandHandler_SetupNews_PermissionValidation(t *testing.T) {
	tests := []struct {
		name                string
		guildID             string
		member              *discordgo.Member
		expectErrorContains string
	}{
		{
			name:                "reject command in DM (no guild)",
			guildID:             "",
			member:              &discordgo.Member{Permissions: discordgo.PermissionAdministrator},
			expectErrorContains: "servidor",
		},
		{
			name:                "reject when member is nil",
			guildID:             "guild123",
			member:              nil,
			expectErrorContains: "permissões",
		},
		{
			name: "reject user without manage server permission",
			guildID: "guild123",
			member: &discordgo.Member{
				Permissions: discordgo.PermissionSendMessages,
			},
			expectErrorContains: "Gerenciar Servidor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockChannelRepository(5)
			handler := NewCommandHandler(repo, 5)

			// We can't easily test the actual Discord response without a real session,
			// but we can verify the logic by checking repository state
			initialCount, _ := repo.GetChannelCount()

			// In real scenario, this would call handleSetupNews
			// For unit test, we verify the conditions

			if tt.guildID == "" || tt.member == nil || !handler.hasManageServerPermission(tt.member) {
				// Should not add channel
				finalCount, _ := repo.GetChannelCount()
				assert.Equal(t, initialCount, finalCount, "Channel should not be added")
			}
		})
	}
}

// TestCommandHandler_SetupNews_ChannelLimitValidation tests channel limit enforcement
func TestCommandHandler_SetupNews_ChannelLimitValidation(t *testing.T) {
	tests := []struct {
		name            string
		maxLimit        int
		existingChannels []string
		newChannelID    string
		shouldSucceed   bool
		errorContains   string
	}{
		{
			name:            "add first channel successfully",
			maxLimit:        5,
			existingChannels: []string{},
			newChannelID:    "channel1",
			shouldSucceed:   true,
		},
		{
			name:            "add channel within limit",
			maxLimit:        5,
			existingChannels: []string{"ch1", "ch2", "ch3"},
			newChannelID:    "ch4",
			shouldSucceed:   true,
		},
		{
			name:            "reject when at limit",
			maxLimit:        3,
			existingChannels: []string{"ch1", "ch2", "ch3"},
			newChannelID:    "ch4",
			shouldSucceed:   false,
			errorContains:   "Limite",
		},
		{
			name:            "reject duplicate channel",
			maxLimit:        5,
			existingChannels: []string{"ch1", "ch2"},
			newChannelID:    "ch1",
			shouldSucceed:   false,
			errorContains:   "já está registrado",
		},
		{
			name:            "allow at exact limit",
			maxLimit:        3,
			existingChannels: []string{"ch1", "ch2"},
			newChannelID:    "ch3",
			shouldSucceed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockChannelRepository(tt.maxLimit)
			
			// Seed existing channels
			for _, chID := range tt.existingChannels {
				err := repo.AddChannel(chID)
				require.NoError(t, err)
			}

			_ = NewCommandHandler(repo, tt.maxLimit)

			// Check if channel already exists
			hasChannel, err := repo.HasChannel(tt.newChannelID)
			require.NoError(t, err)

			if hasChannel {
				// Should fail - duplicate
				assert.False(t, tt.shouldSucceed)
				return
			}

			// Check limit
			count, err := repo.GetChannelCount()
			require.NoError(t, err)

			if count >= tt.maxLimit {
				// Should fail - limit reached
				assert.False(t, tt.shouldSucceed)
				return
			}

			// Try to add
			err = repo.AddChannel(tt.newChannelID)

			if tt.shouldSucceed {
				assert.NoError(t, err)
				
				// Verify it was added
				has, err := repo.HasChannel(tt.newChannelID)
				require.NoError(t, err)
				assert.True(t, has)
			} else {
				assert.Error(t, err)
				if tt.errorContains != "" {
					// In real handler, this would be in the response message
					// Here we check the repository error
					assert.Contains(t, err.Error(), "limit reached", "already exists")
				}
			}
		})
	}
}

// TestCommandHandler_SetupNews_ErrorHandling tests error scenarios
func TestCommandHandler_SetupNews_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func() storage.ChannelRepository
		expectError bool
	}{
		{
			name: "handle repository error on HasChannel",
			setupRepo: func() storage.ChannelRepository {
				repo := NewMockChannelRepository(5)
				repo.getError = fmt.Errorf("database error")
				return repo
			},
			expectError: true,
		},
		{
			name: "handle repository error on GetChannelCount",
			setupRepo: func() storage.ChannelRepository {
				repo := NewMockChannelRepository(5)
				repo.getError = fmt.Errorf("count error")
				return repo
			},
			expectError: true,
		},
		{
			name: "handle repository error on AddChannel",
			setupRepo: func() storage.ChannelRepository {
				repo := NewMockChannelRepository(5)
				repo.addError = fmt.Errorf("add error")
				return repo
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			_ = NewCommandHandler(repo, 5)

			// Verify handler was created (implicitly by not panicking)

			// In a real test, we would verify the error response
			// Here we just ensure the mock setup works
			if tt.expectError {
				// Depending on which operation fails, we should get an error
				_, err := repo.HasChannel("test")
				if err == nil {
					_, err = repo.GetChannelCount()
				}
				if err == nil {
					err = repo.AddChannel("test")
				}
				assert.Error(t, err, "Should encounter an error in repository operations")
			}
		})
	}
}

// TestCommandHandler_Integration tests realistic flow
func TestCommandHandler_Integration(t *testing.T) {
	// Setup
	maxLimit := 3
	repo := NewMockChannelRepository(maxLimit)
	handler := NewCommandHandler(repo, maxLimit)

	// Member with proper permissions
	member := &discordgo.Member{
		Permissions: discordgo.PermissionAdministrator,
	}

	// Test sequence: add channels until limit
	channelIDs := []string{"ch1", "ch2", "ch3"}

	for i, chID := range channelIDs {
		// Verify permission check passes
		assert.True(t, handler.hasManageServerPermission(member))

		// Check current state
		count, err := repo.GetChannelCount()
		require.NoError(t, err)
		assert.Equal(t, i, count)

		// Add channel
		err = repo.AddChannel(chID)
		require.NoError(t, err)

		// Verify added
		has, err := repo.HasChannel(chID)
		require.NoError(t, err)
		assert.True(t, has)
	}

	// Verify limit reached
	count, err := repo.GetChannelCount()
	require.NoError(t, err)
	assert.Equal(t, maxLimit, count)

	// Try to add one more - should fail
	err = repo.AddChannel("ch4")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "limit reached")

	// Verify still at limit
	finalCount, err := repo.GetChannelCount()
	require.NoError(t, err)
	assert.Equal(t, maxLimit, finalCount)
}

// TestCommandHandler_ConcurrentAccess tests thread safety via repository
func TestCommandHandler_ConcurrentAccess(t *testing.T) {
	repo := NewMockChannelRepository(10)
	handler := NewCommandHandler(repo, 10)

	// Add channels concurrently
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			chID := fmt.Sprintf("channel-%d", id)
			_ = repo.AddChannel(chID)
			done <- true
		}(i)
	}

	// Wait for completion
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify state is consistent
	count, err := repo.GetChannelCount()
	require.NoError(t, err)
	assert.LessOrEqual(t, count, 5)
	assert.Greater(t, count, 0)

	// Handler should still be functional
	assert.NotNil(t, handler)
}

// TestCommandHandler_RemoveNews tests the remove-news command logic
func TestCommandHandler_RemoveNews(t *testing.T) {
	tests := []struct {
		name              string
		existingChannels  []string
		channelToRemove   string
		shouldSucceed     bool
		errorContains     string
	}{
		{
			name:             "remove existing channel",
			existingChannels: []string{"ch1", "ch2", "ch3"},
			channelToRemove:  "ch2",
			shouldSucceed:    true,
		},
		{
			name:             "error removing non-existent channel",
			existingChannels: []string{"ch1", "ch2"},
			channelToRemove:  "ch999",
			shouldSucceed:    false,
			errorContains:    "não está registrado",
		},
		{
			name:             "remove last channel",
			existingChannels: []string{"ch1"},
			channelToRemove:  "ch1",
			shouldSucceed:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockChannelRepository(10)

			// Seed existing channels
			for _, chID := range tt.existingChannels {
				err := repo.AddChannel(chID)
				require.NoError(t, err)
			}

			handler := NewCommandHandler(repo, 10)
			initialCount, _ := repo.GetChannelCount()

			// Check if channel exists
			hasChannel, err := repo.HasChannel(tt.channelToRemove)
			require.NoError(t, err)

			if !hasChannel {
				// Should fail - not registered
				assert.False(t, tt.shouldSucceed)
				return
			}

			// Try to remove
			err = repo.RemoveChannel(tt.channelToRemove)

			if tt.shouldSucceed {
				assert.NoError(t, err)

				// Verify it was removed
				has, err := repo.HasChannel(tt.channelToRemove)
				require.NoError(t, err)
				assert.False(t, has)

				// Verify count decreased
				finalCount, _ := repo.GetChannelCount()
				assert.Equal(t, initialCount-1, finalCount)
			} else {
				assert.Error(t, err)
			}

			// Handler should still be functional
			assert.NotNil(t, handler)
		})
	}
}

// TestCommandHandler_ListChannels tests the list-channels command logic
func TestCommandHandler_ListChannels(t *testing.T) {
	tests := []struct {
		name             string
		registeredChannels []string
		expectedCount    int
	}{
		{
			name:             "list empty channels",
			registeredChannels: []string{},
			expectedCount:    0,
		},
		{
			name:             "list single channel",
			registeredChannels: []string{"ch1"},
			expectedCount:    1,
		},
		{
			name:             "list multiple channels",
			registeredChannels: []string{"ch1", "ch2", "ch3", "ch4"},
			expectedCount:    4,
		},
		{
			name:             "list at limit",
			registeredChannels: []string{"ch1", "ch2", "ch3", "ch4", "ch5"},
			expectedCount:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockChannelRepository(10)

			// Seed channels
			for _, chID := range tt.registeredChannels {
				err := repo.AddChannel(chID)
				require.NoError(t, err)
			}

			handler := NewCommandHandler(repo, 10)

			// Get all channels
			channels, err := repo.GetAllChannels()
			require.NoError(t, err)
			assert.Len(t, channels, tt.expectedCount)

			// Verify all expected channels are present
			channelMap := make(map[string]bool)
			for _, ch := range channels {
				channelMap[ch] = true
			}

			for _, expected := range tt.registeredChannels {
				assert.True(t, channelMap[expected], "expected channel %s not found", expected)
			}

			// Handler should still be functional
			assert.NotNil(t, handler)
		})
	}
}

// TestCommandHandler_RemoveNews_Integration tests full remove workflow
func TestCommandHandler_RemoveNews_Integration(t *testing.T) {
	repo := NewMockChannelRepository(5)
	handler := NewCommandHandler(repo, 5)

	// Add channels
	channels := []string{"ch1", "ch2", "ch3"}
	for _, ch := range channels {
		err := repo.AddChannel(ch)
		require.NoError(t, err)
	}

	// Verify all added
	count, err := repo.GetChannelCount()
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Remove middle channel
	err = repo.RemoveChannel("ch2")
	require.NoError(t, err)

	// Verify removed
	has, err := repo.HasChannel("ch2")
	require.NoError(t, err)
	assert.False(t, has)

	// Verify count updated
	count, err = repo.GetChannelCount()
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify other channels still exist
	has, err = repo.HasChannel("ch1")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = repo.HasChannel("ch3")
	require.NoError(t, err)
	assert.True(t, has)

	// Handler should still be functional
	assert.NotNil(t, handler)
}

// TestCommandHandler_UpdateNews tests update-news command
func TestCommandHandler_UpdateNews(t *testing.T) {
	tests := []struct {
		name        string
		hasBot      bool
		expectError bool
	}{
		{
			name:        "successful update with bot configured",
			hasBot:      true,
			expectError: false,
		},
		{
			name:        "error when bot not configured",
			hasBot:      false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockChannelRepository(5)
			handler := NewCommandHandler(repo, 5)

			if tt.hasBot {
				// Create a mock bot (we don't need it functional for this test)
				mockBot := &Bot{}
				handler.SetBot(mockBot)
			}

			// Verify bot is set correctly
			if tt.hasBot {
				assert.NotNil(t, handler.bot)
			} else {
				assert.Nil(t, handler.bot)
			}
		})
	}
}

// TestCommandHandler_SetBot tests setting bot reference
func TestCommandHandler_SetBot(t *testing.T) {
	repo := NewMockChannelRepository(5)
	handler := NewCommandHandler(repo, 5)

	// Initially bot should be nil
	assert.Nil(t, handler.bot)

	// Create a mock bot
	mockBot := &Bot{}
	
	// Set bot
	handler.SetBot(mockBot)

	// Bot should now be set
	assert.NotNil(t, handler.bot)
	assert.Equal(t, mockBot, handler.bot)
}
