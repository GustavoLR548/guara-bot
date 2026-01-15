package bot

import (
"fmt"
"log"
"time"

"github.com/GustavoLR548/godot-news-bot/internal/storage"
"github.com/bwmarrin/discordgo"
)

// RSS Feed Management Commands
// This file contains all RSS feed command handlers
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

	// Verify channel is in the same guild
	if channelValue.GuildID != i.GuildID {
		h.respondError(s, i, "❌ Channel must be in this server.")
		return
	}

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

	// Get all RSS channels
	rssChannels, err := h.channelRepo.GetAllChannels()
	if err != nil {
		log.Printf("Error getting RSS channels: %v", err)
		h.respondError(s, i, "Error fetching channels.")
		return
	}

	// Get all GitHub repositories and their channels
	githubRepos, err := h.githubRepo.GetAllRepositories()
	if err != nil {
		log.Printf("Error getting GitHub repos: %v", err)
		// Continue even if GitHub query fails
		githubRepos = nil
	}

	// Build channel map for GitHub repos
	githubChannelMap := make(map[string][]string) // channelID -> []repoIDs
	for _, repo := range githubRepos {
		channels, err := h.githubRepo.GetRepoChannels(repo.ID)
		if err == nil {
			for _, channelID := range channels {
				githubChannelMap[channelID] = append(githubChannelMap[channelID], repo.ID)
			}
		}
	}

	// Check if we have any subscriptions
	if len(rssChannels) == 0 && len(githubChannelMap) == 0 {
		h.respondSuccess(s, i, "📋 **No registered channels**\n\nUse `/setup-news` for RSS feeds or `/setup-repo-channel` for GitHub repositories.")
		return
	}

	// Build response
	var response string
	response = "📋 **Registered Channels**\n\n"

	// List RSS channels
	if len(rssChannels) > 0 {
		response += fmt.Sprintf("**📰 RSS News** (%d channels):\n", len(rssChannels))
		for i, channelID := range rssChannels {
			response += fmt.Sprintf("%d. <#%s>\n", i+1, channelID)
		}
		response += "\n"
	}

	// List GitHub channels
	if len(githubChannelMap) > 0 {
		response += fmt.Sprintf("**🐙 GitHub PRs** (%d channels):\n", len(githubChannelMap))
		idx := 1
		for channelID, repoIDs := range githubChannelMap {
			response += fmt.Sprintf("%d. <#%s> (repos: %s)\n", idx, channelID, joinStrings(repoIDs, ", "))
			idx++
		}
		response += "\n"
	}

	response += "💡 Use `/remove-news` for RSS or `/remove-repo-channel` for GitHub subscriptions."

	h.respondSuccess(s, i, response)

	log.Printf("Channel list requested by user in guild %s", i.GuildID)
}

// joinStrings is a simple helper to join strings with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
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

	// Check rate limit
	if limited, remaining := updateNewsRateLimiter.Check(member.User.ID); limited {
		h.respondError(s, i, fmt.Sprintf("⏳ Please wait %d seconds before triggering another update.", int(remaining.Seconds())))
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
	updateNewsRateLimiter.Record(member.User.ID) // Record the command use
	
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

	// Check rate limit
	if limited, remaining := updateNewsRateLimiter.Check(member.User.ID); limited {
		h.respondError(s, i, fmt.Sprintf("⏳ Please wait %d seconds before triggering another update.", int(remaining.Seconds())))
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
	updateNewsRateLimiter.Record(member.User.ID) // Record the command use
	
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

	// Validate feed ID
	if err := isValidFeedID(feedID); err != nil {
		h.respondError(s, i, fmt.Sprintf("❌ Invalid feed ID: %s", err.Error()))
		return
	}

	// Validate feed URL
	if err := isValidURL(feedURL); err != nil {
		h.respondError(s, i, fmt.Sprintf("❌ Invalid feed URL: %s", err.Error()))
		return
	}

	// Validate title length (Discord embed title limit is 256)
	if len(title) > 256 {
		h.respondError(s, i, "❌ Feed title too long (max 256 characters)")
		return
	}

	// Validate description length (Discord embed description limit is 4096)
	if len(description) > 4096 {
		h.respondError(s, i, "❌ Feed description too long (max 4096 characters)")
		return
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

	// Cleanup: Remove channel associations and orphaned data
	if len(channels) > 0 {
		log.Printf("Cleaning up %d channel associations for feed %s", len(channels), feedID)
		for _, chID := range channels {
			if err := h.channelRepo.RemoveChannel(chID, feedID); err != nil {
				log.Printf("Error removing feed %s from channel %s: %v", feedID, chID, err)
			}
		}
	}

	// Clear pending queue - feedRepo.ClearPending doesn't exist, so we'll leave queue as is
	// The queue will naturally clear when articles are fetched next time
	// or can be cleared manually via Redis CLI if needed

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

	// Validate schedule times
	if err := validateScheduleTimes(times); err != nil {
		h.respondError(s, i, fmt.Sprintf("❌ %s", err.Error()))
		return
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
