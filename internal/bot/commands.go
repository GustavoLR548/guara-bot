package bot

import (
	"fmt"
	"log"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// CommandHandler handles Discord slash commands
type CommandHandler struct {
	channelRepo storage.ChannelRepository
	feedRepo    storage.FeedRepository
	maxLimit    int
	bot         *Bot // Reference to bot for triggering updates
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(channelRepo storage.ChannelRepository, feedRepo storage.FeedRepository, maxLimit int) *CommandHandler {
	return &CommandHandler{
		channelRepo: channelRepo,
		feedRepo:    feedRepo,
		maxLimit:    maxLimit,
	}
}

// SetBot sets the bot reference (called after bot is created)
func (h *CommandHandler) SetBot(bot *Bot) {
	h.bot = bot
}

// RegisterCommands registers all slash commands with Discord
func (h *CommandHandler) RegisterCommands(s *discordgo.Session) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "setup-news",
			Description: "Configure a channel to receive news from a specific feed",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "The channel to setup for news updates",
					Required:    true,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "feed",
					Description: "The feed identifier (default: godot-official)",
					Required:    false,
				},
			},
		},
		{
			Name:        "remove-news",
			Description: "Remove a channel from receiving news from a specific feed",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "The channel to remove from news updates",
					Required:    true,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "feed",
					Description: "The feed identifier to unsubscribe from",
					Required:    false,
				},
			},
		},
		{
			Name:        "list-channels",
			Description: "List all channels registered for news updates (Admin only)",
		},
		{
			Name:        "update-news",
			Description: "Force an immediate check for new articles from a specific feed (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "feed",
					Description: "The feed identifier to check (default: godot-official)",
					Required:    false,
				},
			},
		},
		{
			Name:        "update-all-news",
			Description: "Force an immediate check for all registered feeds (Admin only)",
		},
		{
			Name:        "register-feed",
			Description: "Register a new RSS feed (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "identifier",
					Description: "Unique identifier for the feed (e.g., godot-weekly)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "url",
					Description: "RSS feed URL",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "title",
					Description: "Feed title",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "description",
					Description: "Feed description",
					Required:    false,
				},
			},
		},
		{
			Name:        "unregister-feed",
			Description: "Unregister an RSS feed (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "identifier",
					Description: "The feed identifier to unregister",
					Required:    true,
				},
			},
		},
		{
			Name:        "list-feeds",
			Description: "List all registered RSS feeds",
		},
		{
			Name:        "schedule-feed",
			Description: "Set check times for a feed (Admin only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "identifier",
					Description: "The feed identifier",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "times",
					Description: "Check times in 24h format, comma-separated (e.g., 09:00,13:00,18:00)",
					Required:    true,
				},
			},
		},
		{
			Name:        "set-language",
			Description: "Set the default language for news summaries in this server",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "language",
					Description: "Select language",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "🇧🇷 Português (Brasil)", Value: "pt-BR"},
						{Name: "🇺🇸 English", Value: "en"},
						{Name: "🇪🇸 Español", Value: "es"},
						{Name: "🇫🇷 Français", Value: "fr"},
						{Name: "🇩🇪 Deutsch", Value: "de"},
						{Name: "🇯🇵 日本語", Value: "ja"},
					},
				},
			},
		},
		{
			Name:        "set-channel-language",
			Description: "Set a specific language for news summaries in a channel",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "The channel to configure",
					Required:    true,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "language",
					Description: "Select language (leave empty to use server default)",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "🇧🇷 Português (Brasil)", Value: "pt-BR"},
						{Name: "🇺🇸 English", Value: "en"},
						{Name: "🇪🇸 Español", Value: "es"},
						{Name: "🇫🇷 Français", Value: "fr"},
						{Name: "🇩🇪 Deutsch", Value: "de"},
						{Name: "🇯🇵 日本語", Value: "ja"},
					},
				},
			},
		},
		{
			Name:        "help",
			Description: "Show all available commands and how to use them",
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			return fmt.Errorf("failed to create command %s: %w", cmd.Name, err)
		}
		log.Printf("Registered command: %s", cmd.Name)
	}

	return nil
}

// HandleCommands sets up the command handler
func (h *CommandHandler) HandleCommands(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.ApplicationCommandData().Name {
		case "setup-news":
			h.handleSetupNews(s, i)
		case "remove-news":
			h.handleRemoveNews(s, i)
		case "list-channels":
			h.handleListChannels(s, i)
		case "update-news":
			h.handleUpdateNews(s, i)
		case "update-all-news":
			h.handleUpdateAllNews(s, i)
		case "register-feed":
			h.handleRegisterFeed(s, i)
		case "unregister-feed":
			h.handleUnregisterFeed(s, i)
		case "list-feeds":
			h.handleListFeeds(s, i)
		case "schedule-feed":
			h.handleScheduleFeed(s, i)
		case "set-language":
			h.handleSetLanguage(s, i)
		case "set-channel-language":
			h.handleSetChannelLanguage(s, i)
		case "help":
			h.handleHelp(s, i)
		}
	})
}

// handleSetupNews handles the /setup-news command
func (h *CommandHandler) handleSetupNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[SETUP-NEWS] Command triggered by user %s in guild %s", i.Member.User.ID, i.GuildID)
	
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		log.Printf("[SETUP-NEWS] ERROR: Command used outside guild")
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		log.Printf("[SETUP-NEWS] ERROR: Could not get member")
		h.respondError(s, i, "Could not verify your permissions.")
		return
	}

	// Check if user has "Manage Server" permission
	if !h.hasManageServerPermission(member) {
		log.Printf("[SETUP-NEWS] ERROR: User %s lacks Manage Server permission", member.User.ID)
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get the channel parameter
	options := i.ApplicationCommandData().Options
	log.Printf("[SETUP-NEWS] Received %d options", len(options))
	if len(options) == 0 {
		log.Printf("[SETUP-NEWS] ERROR: No channel option provided")
		h.respondError(s, i, "❌ You need to specify a channel.")
		return
	}

	// Extract channel ID from the option
	channelValue := options[0].ChannelValue(s)
	if channelValue == nil {
		log.Printf("[SETUP-NEWS] ERROR: Failed to get channel value")
		h.respondError(s, i, "❌ Invalid channel.")
		return
	}
	
	channelID := channelValue.ID
	log.Printf("[SETUP-NEWS] Channel selected: %s (ID: %s, Type: %d)", channelValue.Name, channelID, channelValue.Type)

	// Get feed identifier (default to godot-official)
	feedID := "godot-official"
	if len(options) > 1 {
		feedID = options[1].StringValue()
		log.Printf("[SETUP-NEWS] Custom feed ID provided: %s", feedID)
	} else {
		log.Printf("[SETUP-NEWS] Using default feed ID: %s", feedID)
	}

	// Respond immediately to avoid timeout
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("[SETUP-NEWS] ERROR: Failed to send initial response: %v", err)
		return
	}

	// Now do the work
	log.Printf("[SETUP-NEWS] Checking if feed exists: %s", feedID)
	hasFeed, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("[SETUP-NEWS] ERROR: Failed to check feed existence: %v", err)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Error checking feed.",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}
	if !hasFeed {
		log.Printf("[SETUP-NEWS] ERROR: Feed not found: %s", feedID)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Feed '%s' not found. Use `/list-feeds` to see available feeds.", feedID),
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}
	log.Printf("[SETUP-NEWS] Feed exists: %s", feedID)

	// Verify it's a text channel
	if channelValue.Type != discordgo.ChannelTypeGuildText {
		log.Printf("[SETUP-NEWS] ERROR: Invalid channel type: %d (expected %d)", channelValue.Type, discordgo.ChannelTypeGuildText)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Only text channels can receive news.",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	// Check if channel is already subscribed to this feed
	log.Printf("[SETUP-NEWS] Checking if channel %s is already subscribed to feed %s", channelID, feedID)
	feeds, err := h.channelRepo.GetChannelFeeds(channelID)
	if err != nil {
		log.Printf("[SETUP-NEWS] ERROR: Failed to get channel feeds: %v", err)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Error checking channel.",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}
	log.Printf("[SETUP-NEWS] Channel %s has %d feeds: %v", channelID, len(feeds), feeds)

	for _, f := range feeds {
		if f == feedID {
			log.Printf("[SETUP-NEWS] ERROR: Channel %s already subscribed to feed %s", channelID, feedID)
			s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: fmt.Sprintf("⚠️ This channel is already subscribed to feed '%s'.", feedID),
				Flags:   discordgo.MessageFlagsEphemeral,
			})
			return
		}
	}

	// Check if limit would be exceeded (count unique channels)
	log.Printf("[SETUP-NEWS] Checking channel count")
	count, err := h.channelRepo.GetChannelCount()
	if err != nil {
		log.Printf("[SETUP-NEWS] ERROR: Failed to get channel count: %v", err)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Error checking channel limit.",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}
	log.Printf("[SETUP-NEWS] Current channel count: %d, Max: %d, New channel: %v", count, h.maxLimit, len(feeds) == 0)

	if count >= h.maxLimit && len(feeds) == 0 {
		// Only enforce limit for new channels, not for adding feeds to existing channels
		log.Printf("[SETUP-NEWS] ERROR: Channel limit reached (%d/%d)", count, h.maxLimit)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Channel limit reached (%d/%d). Cannot add more channels.", count, h.maxLimit),
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	// Add the channel-feed association
	log.Printf("[SETUP-NEWS] Adding channel %s for feed %s", channelID, feedID)
	if err := h.channelRepo.AddChannel(channelID, feedID); err != nil {
		log.Printf("[SETUP-NEWS] ERROR: Failed to add channel: %v", err)
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "❌ Error registering channel.",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}
	log.Printf("[SETUP-NEWS] SUCCESS: Channel added")

	// Get feed info for response
	feed, err := h.feedRepo.GetFeed(feedID)
	if err != nil {
		log.Printf("[SETUP-NEWS] Warning: Failed to get feed details: %v", err)
		// Continue anyway, feed exists
		feed = &storage.Feed{ID: feedID, Title: feedID}
	}

	// Send success message
	s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("✅ **Channel configured successfully!**\n\n<#%s> will now receive news from **%s** (%s).", channelID, feed.Title, feedID),
		Flags:   discordgo.MessageFlagsEphemeral,
	})

	log.Printf("[SETUP-NEWS] SUCCESS: Channel %s subscribed to feed %s in guild %s", channelID, feedID, i.GuildID)
}

// handleRemoveNews handles the /remove-news command
func (h *CommandHandler) handleRemoveNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Could not verify your permissions.")
		return
	}

	// Check if user has "Manage Server" permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get the channel parameter
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondError(s, i, "❌ You need to specify a channel.")
		return
	}

	// Extract channel ID from the option
	channelValue := options[0].ChannelValue(s)
	if channelValue == nil {
		h.respondError(s, i, "❌ Invalid channel.")
		return
	}
	
	channelID := channelValue.ID

	// Get feed identifier (optional)
	var feedID string
	if len(options) > 1 {
		feedID = options[1].StringValue()
	}

	// Get channel's feeds
	feeds, err := h.channelRepo.GetChannelFeeds(channelID)
	if err != nil {
		log.Printf("Error checking channel feeds: %v", err)
		h.respondError(s, i, "Error checking channel.")
		return
	}

	if len(feeds) == 0 {
		h.respondError(s, i, "⚠️ This channel is not subscribed to any feed.")
		return
	}

	// If no feed specified and channel has multiple feeds, show them
	if feedID == "" && len(feeds) > 1 {
		feedList := ""
		for _, f := range feeds {
			feedList += fmt.Sprintf("• %s\n", f)
		}
		h.respondError(s, i, fmt.Sprintf("⚠️ This channel is subscribed to multiple feeds. Specify which one to remove:\n\n%s", feedList))
		return
	}

	// If not specified, use the only feed
	if feedID == "" {
		feedID = feeds[0]
	}

	// Verify channel is subscribed to this feed
	isSubscribed := false
	for _, f := range feeds {
		if f == feedID {
			isSubscribed = true
			break
		}
	}

	if !isSubscribed {
		h.respondError(s, i, fmt.Sprintf("⚠️ This channel is not subscribed to feed '%s'.", feedID))
		return
	}

	// Remove the channel-feed association
	if err := h.channelRepo.RemoveChannel(channelID, feedID); err != nil {
		log.Printf("Error removing channel: %v", err)
		h.respondError(s, i, "Error removing channel.")
		return
	}

	// Success response
	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Channel removed successfully!**\n\n<#%s> will no longer receive news from **%s**.",
		channelID,
		feedID,
	))

	log.Printf("Channel %s unsubscribed from feed %s in guild %s", channelID, feedID, i.GuildID)
}

// handleListChannels handles the /list-channels command
func (h *CommandHandler) handleListChannels(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Could not verify your permissions.")
		return
	}

	// Check if user has "Manage Server" permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get all channels
	channels, err := h.channelRepo.GetAllChannels()
	if err != nil {
		log.Printf("Error getting channels: %v", err)
		h.respondError(s, i, "Error fetching channels.")
		return
	}

	if len(channels) == 0 {
		h.respondSuccess(s, i, "📋 **No registered channels**\n\nUse `/setup-news` in a channel to start receiving news.")
		return
	}

	// Build channel list
	var response string
	response = fmt.Sprintf("📋 **Registered Channels** (%d/%d)\n\n", len(channels), h.maxLimit)

	for i, channelID := range channels {
		// Use simple channel mention format
		response += fmt.Sprintf("%d. <#%s>\n", i+1, channelID)
	}

	response += fmt.Sprintf("\n💡 Use `/remove-news` in a channel to remove it from the list.")

	h.respondSuccess(s, i, response)

	log.Printf("Channel list requested by user in guild %s", i.GuildID)
}

// handleUpdateNews handles the /update-news command (specific feed)
func (h *CommandHandler) handleUpdateNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Could not verify your permissions.")
		return
	}

	// Check for Manage Server permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the 'Manage Server' permission to use this command.")
		return
	}

	// Get feed identifier (default to godot-official)
	feedID := "godot-official"
	options := i.ApplicationCommandData().Options
	if len(options) > 0 {
		feedID = options[0].StringValue()
	}

	// Verify feed exists
	hasFeed, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "❌ Error checking feed.")
		return
	}
	if !hasFeed {
		h.respondError(s, i, fmt.Sprintf("❌ Feed not found: %s\n\nUse `/list-feeds` to see available feeds.", feedID))
		return
	}

	// Check if feed has any channels subscribed
	channels, err := h.channelRepo.GetFeedChannels(feedID)
	if err != nil {
		log.Printf("Error getting feed channels: %v", err)
		h.respondError(s, i, "❌ Error checking registered channels.")
		return
	}

	if len(channels) == 0 {
		h.respondError(s, i, fmt.Sprintf("⚠️ No channels registered for feed: %s\n\nUse `/setup-news` to configure a channel first.", feedID))
		return
	}

	// Check if bot is set
	if h.bot == nil {
		h.respondError(s, i, "❌ Bot is not configured correctly.")
		return
	}

	// Get feed info
	feed, err := h.feedRepo.GetFeed(feedID)
	if err != nil {
		log.Printf("Error getting feed: %v", err)
		// Continue anyway
	}

	// Send initial response
	respondErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔄 Checking for updates from **%s**...", feed.Title),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if respondErr != nil {
		log.Printf("Error sending initial response: %v", respondErr)
		return
	}

	// Trigger immediate news check for specific feed
	go func() {
		result := h.bot.CheckAndPostFeedNews(feedID)
		
		var followupMessage string
		if result {
			followupMessage = fmt.Sprintf("✅ Check for **%s** completed! If there are any updates, they were posted to registered channels.", feed.Title)
		} else {
			followupMessage = fmt.Sprintf("ℹ️ No updates found in **%s** or there was an error. Check logs for more details.", feed.Title)
		}

		// Send follow-up message
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: followupMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		if err != nil {
			log.Printf("Error sending followup message: %v", err)
		}
	}()

	log.Printf("Manual news update triggered for feed %s by user in guild %s", feedID, i.GuildID)
}

// handleUpdateAllNews handles the /update-all-news command (all feeds)
func (h *CommandHandler) handleUpdateAllNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Could not verify your permissions.")
		return
	}

	// Check for Manage Server permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the 'Manage Server' permission to use this command.")
		return
	}

	// Check if there are any channels registered
	count, err := h.channelRepo.GetChannelCount()
	if err != nil {
		log.Printf("Error getting channel count: %v", err)
		h.respondError(s, i, "❌ Error checking registered channels.")
		return
	}

	if count == 0 {
		h.respondError(s, i, "⚠️ No channels registered to receive news.\n\nUse `/setup-news` to configure a channel first.")
		return
	}

	// Check if bot is set
	if h.bot == nil {
		h.respondError(s, i, "❌ Bot is not configured correctly.")
		return
	}

	// Send initial response
	respondErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🔄 Checking for updates from all feeds...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if respondErr != nil {
		log.Printf("Error sending initial response: %v", respondErr)
		return
	}

	// Trigger immediate news check for all feeds
	go func() {
		result := h.bot.CheckAndPostNews()
		
		var followupMessage string
		if result {
			followupMessage = "✅ Check for all feeds completed! If there are any updates, they were posted to registered channels."
		} else {
			followupMessage = "ℹ️ No updates found or there was an error. Check logs for more details."
		}

		// Send follow-up message
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: followupMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		if err != nil {
			log.Printf("Error sending followup message: %v", err)
		}
	}()

	log.Printf("Manual news update for all feeds triggered by user in guild %s", i.GuildID)
}

// handleRegisterFeed handles the /register-feed command
func (h *CommandHandler) handleRegisterFeed(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check guild
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Check permissions
	member := i.Member
	if member == nil || !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get parameters
	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.respondError(s, i, "❌ You need to specify identifier and url.")
		return
	}

	feedID := options[0].StringValue()
	feedURL := options[1].StringValue()
	
	title := feedID
	if len(options) > 2 && options[2].StringValue() != "" {
		title = options[2].StringValue()
	}
	
	description := ""
	if len(options) > 3 {
		description = options[3].StringValue()
	}

	// Check if feed already exists
	exists, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "Error checking feed.")
		return
	}

	if exists {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' is already registered.", feedID))
		return
	}

	// Register feed
	feed := storage.Feed{
		ID:          feedID,
		URL:         feedURL,
		Title:       title,
		Description: description,
		AddedAt:     time.Now(),
	}

	if err := h.feedRepo.RegisterFeed(feed); err != nil {
		log.Printf("Error registering feed: %v", err)
		h.respondError(s, i, "Error registering feed.")
		return
	}

	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Feed registered successfully!**\n\n**ID:** %s\n**Title:** %s\n**URL:** %s",
		feedID, title, feedURL,
	))

	log.Printf("Feed %s registered in guild %s", feedID, i.GuildID)
}

// handleUnregisterFeed handles the /unregister-feed command
func (h *CommandHandler) handleUnregisterFeed(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check guild
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Check permissions
	member := i.Member
	if member == nil || !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get parameter
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondError(s, i, "❌ You need to specify the feed identifier.")
		return
	}

	feedID := options[0].StringValue()

	// Check if feed exists
	exists, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "Error checking feed.")
		return
	}

	if !exists {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' not found.", feedID))
		return
	}

	// Check how many channels are using this feed
	channels, err := h.channelRepo.GetFeedChannels(feedID)
	if err != nil {
		log.Printf("Error getting feed channels: %v", err)
	}

	// Unregister feed
	if err := h.feedRepo.UnregisterFeed(feedID); err != nil {
		log.Printf("Error unregistering feed: %v", err)
		h.respondError(s, i, "Error removing feed.")
		return
	}

	// If channels were using this feed, remove associations
	if len(channels) > 0 {
		for _, chID := range channels {
			_ = h.channelRepo.RemoveChannel(chID, feedID)
		}
	}

	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Feed removed successfully!**\n\nFeed '%s' was removed%s.",
		feedID,
		func() string {
			if len(channels) > 0 {
				return fmt.Sprintf(" and unlinked from %d channel(s)", len(channels))
			}
			return ""
		}(),
	))

	log.Printf("Feed %s unregistered from guild %s", feedID, i.GuildID)
}

// handleListFeeds handles the /list-feeds command
func (h *CommandHandler) handleListFeeds(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get all feeds
	feeds, err := h.feedRepo.GetAllFeeds()
	if err != nil {
		log.Printf("Error getting feeds: %v", err)
		h.respondError(s, i, "Error listing feeds.")
		return
	}

	if len(feeds) == 0 {
		h.respondError(s, i, "ℹ️ No feeds registered.")
		return
	}

	// Build response
	response := "📰 **Registered Feeds**\n\n"
	for _, feed := range feeds {
		response += fmt.Sprintf("**%s** (`%s`)\n", feed.Title, feed.ID)
		response += fmt.Sprintf("└ URL: %s\n", feed.URL)
		
		// Show schedule if any
		if len(feed.Schedule) > 0 {
			times := ""
			for idx, t := range feed.Schedule {
				if idx > 0 {
					times += ", "
				}
				times += t
			}
			response += fmt.Sprintf("└ Schedule: %s\n", times)
		}
		
		// Show channel count
		channels, err := h.channelRepo.GetFeedChannels(feed.ID)
		if err == nil {
			response += fmt.Sprintf("└ Subscribed channels: %d\n", len(channels))
		}
		response += "\n"
	}

	h.respondSuccess(s, i, response)
}

// handleScheduleFeed handles the /schedule-feed command
func (h *CommandHandler) handleScheduleFeed(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check guild
	if i.GuildID == "" {
		h.respondError(s, i, "This command can only be used in a server.")
		return
	}

	// Check permissions
	member := i.Member
	if member == nil || !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get parameters
	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.respondError(s, i, "❌ You need to specify identifier and times.")
		return
	}

	feedID := options[0].StringValue()
	timesStr := options[1].StringValue()

	// Check if feed exists
	exists, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "Error checking feed.")
		return
	}

	if !exists {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' not found.", feedID))
		return
	}

	// Parse times (comma-separated)
	times := []string{}
	for _, t := range splitAndTrim(timesStr, ",") {
		if t != "" {
			times = append(times, t)
		}
	}

	// Set schedule
	if err := h.feedRepo.SetSchedule(feedID, times); err != nil {
		log.Printf("Error setting schedule: %v", err)
		h.respondError(s, i, fmt.Sprintf("❌ Error setting schedule: %v", err))
		return
	}

	timesDisplay := ""
	for idx, t := range times {
		if idx > 0 {
			timesDisplay += ", "
		}
		timesDisplay += t
	}

	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Schedule configured!**\n\nFeed '%s' will be checked at the following times:\n%s",
		feedID, timesDisplay,
	))

	log.Printf("Schedule set for feed %s: %v", feedID, times)
}

// splitAndTrim splits a string and trims each part
func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for _, part := range splitString(s, sep) {
		trimmed := trimString(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// splitString splits a string by separator
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	
	parts := []string{}
	current := ""
	
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			parts = append(parts, current)
			current = ""
			i += len(sep) - 1
		} else {
			current += string(s[i])
		}
	}
	parts = append(parts, current)
	
	return parts
}

// trimString removes leading and trailing whitespace
func trimString(s string) string {
	start := 0
	end := len(s)
	
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	
	return s[start:end]
}

// hasManageServerPermission checks if a member has Manage Server permission
func (h *CommandHandler) hasManageServerPermission(member *discordgo.Member) bool {
	// MANAGE_GUILD permission value
	const manageGuildPermission int64 = 0x0000000000000020

	// Check member permissions
	permissions := member.Permissions

	// Check if user has administrator permission (grants all permissions)
	if permissions&discordgo.PermissionAdministrator != 0 {
		return true
	}

	// Check if user has manage server permission
	if permissions&manageGuildPermission != 0 {
		return true
	}

	return false
}

// respondError sends an error response to the interaction
func (h *CommandHandler) respondError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral, // Only visible to the user
		},
	})
	if err != nil {
		log.Printf("Error sending error response: %v", err)
	}
}

// respondSuccess sends a success response to the interaction
func (h *CommandHandler) respondSuccess(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
		},
	})
	if err != nil {
		log.Printf("Error sending success response: %v", err)
	}
}

// handleSetLanguage handles the /set-language command
func (h *CommandHandler) handleSetLanguage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[SET-LANGUAGE] Command triggered by user %s in guild %s", i.Member.User.ID, i.GuildID)

	// Defer response immediately to prevent "unknown interaction" error
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[SET-LANGUAGE] ERROR: Failed to send deferred response: %v", err)
		return
	}

	// Check if command was used in a guild
	if i.GuildID == "" {
		h.followUpError(s, i, "This command can only be used in a server.")
		return
	}

	// Check permissions
	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get language parameter
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.followUpError(s, i, "❌ You need to specify a language.")
		return
	}

	languageCode := options[0].StringValue()
	log.Printf("[SET-LANGUAGE] Setting guild %s language to: %s", i.GuildID, languageCode)

	// Save guild language preference
	if err := h.channelRepo.SetGuildLanguage(i.GuildID, languageCode); err != nil {
		log.Printf("[SET-LANGUAGE] ERROR: Failed to save language: %v", err)
		h.followUpError(s, i, fmt.Sprintf("❌ Error saving language preference: %v", err))
		return
	}

	languageFlag := getLanguageFlag(languageCode)
	h.followUpSuccess(s, i, fmt.Sprintf("✅ Server default language set to: %s %s\n\nIndividual channels can have different languages using `/set-channel-language`.", languageFlag, languageCode))
}

// handleSetChannelLanguage handles the /set-channel-language command
func (h *CommandHandler) handleSetChannelLanguage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[SET-CHANNEL-LANGUAGE] Command triggered by user %s in guild %s", i.Member.User.ID, i.GuildID)

	// Defer response immediately
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[SET-CHANNEL-LANGUAGE] ERROR: Failed to send deferred response: %v", err)
		return
	}

	// Check if command was used in a guild
	if i.GuildID == "" {
		h.followUpError(s, i, "This command can only be used in a server.")
		return
	}

	// Check permissions
	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Get parameters
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.followUpError(s, i, "❌ You need to specify a channel.")
		return
	}

	channelValue := options[0].ChannelValue(s)
	if channelValue == nil {
		h.followUpError(s, i, "❌ Invalid channel.")
		return
	}

	channelID := channelValue.ID

	// Check if language is provided
	if len(options) < 2 {
		// Remove channel-specific language (use guild default)
		if err := h.channelRepo.SetChannelLanguage(channelID, ""); err != nil {
			log.Printf("[SET-CHANNEL-LANGUAGE] ERROR: Failed to clear channel language: %v", err)
			h.followUpError(s, i, fmt.Sprintf("❌ Error clearing channel language: %v", err))
			return
		}
		h.followUpSuccess(s, i, fmt.Sprintf("✅ Channel <#%s> will now use the server's default language.", channelID))
		return
	}

	languageCode := options[1].StringValue()
	log.Printf("[SET-CHANNEL-LANGUAGE] Setting channel %s language to: %s", channelID, languageCode)

	// Save channel language preference
	if err := h.channelRepo.SetChannelLanguage(channelID, languageCode); err != nil {
		log.Printf("[SET-CHANNEL-LANGUAGE] ERROR: Failed to save language: %v", err)
		h.followUpError(s, i, fmt.Sprintf("❌ Error saving channel language: %v", err))
		return
	}

	languageFlag := getLanguageFlag(languageCode)
	h.followUpSuccess(s, i, fmt.Sprintf("✅ Channel <#%s> language set to: %s %s", channelID, languageFlag, languageCode))
}

// getLanguageFlag returns the emoji flag for a language code
func getLanguageFlag(code string) string {
	flags := map[string]string{
		"pt-BR": "🇧🇷",
		"en":    "🇺🇸",
		"es":    "🇪🇸",
		"fr":    "🇫🇷",
		"de":    "🇩🇪",
		"ja":    "🇯🇵",
	}
	if flag, ok := flags[code]; ok {
		return flag
	}
	return "🌐"
}

// followUpError sends an error follow-up message
func (h *CommandHandler) followUpError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	_, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: message,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		log.Printf("Error sending follow-up error: %v", err)
	}
}

// followUpSuccess sends a success follow-up message
func (h *CommandHandler) followUpSuccess(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	_, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: message,
	})
	if err != nil {
		log.Printf("Error sending follow-up success: %v", err)
	}
}

// handleHelp handles the /help command
func (h *CommandHandler) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	help := "📚 **Guara Bot - Command Reference**\n\n" +
		"**📰 Feed Management** (Admin)\n" +
		"• `/register-feed` - Register a new RSS feed\n" +
		"• `/unregister-feed` - Remove an RSS feed\n" +
		"• `/list-feeds` - List all registered feeds\n" +
		"• `/schedule-feed` - Set check times for a feed (e.g., 09:00,18:00)\n\n" +
		"**📢 Channel Setup** (Admin)\n" +
		"• `/setup-news` - Subscribe a channel to receive news from a feed\n" +
		"• `/remove-news` - Unsubscribe a channel from a feed\n" +
		"• `/list-channels` - List all channels receiving news updates\n\n" +
		"**🔄 Manual Updates** (Admin)\n" +
		"• `/update-news` - Force immediate check for a specific feed\n" +
		"• `/update-all-news` - Force immediate check for all feeds\n\n" +
		"**🌍 Language Settings** (Admin)\n" +
		"• `/set-language` - Set server default language for summaries\n" +
		"• `/set-channel-language` - Set language for a specific channel\n\n" +
		"**ℹ️ Information**\n" +
		"• `/help` - Show this help message\n\n" +
		"**Supported Languages:** 🇧🇷 Português | 🇺🇸 English | 🇪🇸 Español | 🇫🇷 Français | 🇩🇪 Deutsch | 🇯🇵 日本語\n\n" +
		"💡 **Tip:** Most commands require the **Manage Server** permission."

	h.respondSuccess(s, i, help)
}
