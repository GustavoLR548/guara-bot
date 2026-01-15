package bot

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/github"
	"github.com/bwmarrin/discordgo"
	"log"
)

// handleRegisterRepo handles the /register-repo command
func (h *CommandHandler) handleRegisterRepo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[REGISTER-REPO] Command triggered by user %s in guild %s", i.Member.User.ID, i.GuildID)

	// Defer response
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[REGISTER-REPO] ERROR: Failed to send deferred response: %v", err)
		return
	}

	// Check permissions
	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Parse options
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	repoID := optionMap["id"].StringValue()
	owner := optionMap["owner"].StringValue()
	repoName := optionMap["repo"].StringValue()
	branch := "main"
	if opt, ok := optionMap["branch"]; ok {
		branch = opt.StringValue()
	}

	// Validate inputs
	if repoID == "" || owner == "" || repoName == "" {
		h.followUpError(s, i, "❌ Missing required parameters.")
		return
	}

	// Validate repository ID format
	if err := isValidRepoID(repoID); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Invalid repository ID: %s", err.Error()))
		return
	}

	// Validate owner/repo names
	if err := isValidGitHubName(owner); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Invalid owner name: %s", err.Error()))
		return
	}

	if err := isValidGitHubName(repoName); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Invalid repository name: %s", err.Error()))
		return
	}

	// Validate branch name (basic check)
	if len(branch) > 255 {
		h.followUpError(s, i, "❌ Branch name too long (max 255 characters)")
		return
	}

	// Check if repository already exists
	exists, err := h.githubRepo.HasRepository(repoID)
	if err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Error checking repository: %v", err))
		return
	}
	if exists {
		h.followUpError(s, i, fmt.Sprintf("❌ Repository `%s` is already registered.", repoID))
		return
	}

	// Register repository
	repo := github.Repository{
		ID:           repoID,
		Owner:        owner,
		Name:         repoName,
		TargetBranch: branch,
		AddedAt:      time.Now(),
	}

	if err := h.githubRepo.RegisterRepository(repo); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to register repository: %v", err))
		return
	}

	message := fmt.Sprintf("✅ **Repository Registered**\n"+
		"📦 **ID:** `%s`\n"+
		"👤 **Owner:** `%s`\n"+
		"📁 **Repo:** `%s`\n"+
		"🌿 **Branch:** `%s`\n\n"+
		"Use `/setup-repo-channel` to subscribe channels to PR updates.",
		repoID, owner, repoName, branch)

	h.followUpSuccess(s, i, message)
}

// handleUnregisterRepo handles the /unregister-repo command
func (h *CommandHandler) handleUnregisterRepo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[UNREGISTER-REPO] Command triggered by user %s", i.Member.User.ID)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[UNREGISTER-REPO] ERROR: Failed to send deferred response: %v", err)
		return
	}

	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.followUpError(s, i, "❌ You need to specify a repository ID.")
		return
	}

	repoID := options[0].StringValue()

	// Check if repository exists
	exists, err := h.githubRepo.HasRepository(repoID)
	if err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Error checking repository: %v", err))
		return
	}
	if !exists {
		h.followUpError(s, i, fmt.Sprintf("❌ Repository `%s` not found.", repoID))
		return
	}

	// Get subscribed channels before unregistering (for cleanup and user feedback)
	channels, err := h.githubRepo.GetRepoChannels(repoID)
	if err != nil {
		log.Printf("[UNREGISTER-REPO] Error getting repo channels: %v", err)
	}

	// Unregister repository
	if err := h.githubRepo.UnregisterRepository(repoID); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to unregister repository: %v", err))
		return
	}

	// Cleanup: Remove channel associations and orphaned data
	if len(channels) > 0 {
		log.Printf("[UNREGISTER-REPO] Cleaning up %d channel associations for repo %s", len(channels), repoID)
		for _, channelID := range channels {
			if err := h.githubRepo.RemoveRepoChannel(repoID, channelID); err != nil {
				log.Printf("[UNREGISTER-REPO] Error removing repo %s from channel %s: %v", repoID, channelID, err)
			}
		}
	}

	// Clear pending PRs
	if err := h.githubRepo.ClearPendingQueue(repoID); err != nil {
		log.Printf("[UNREGISTER-REPO] Error clearing pending PRs for repo %s: %v", repoID, err)
	}

	// Success message with warning if channels were affected
	message := fmt.Sprintf("✅ Repository `%s` has been unregistered.", repoID)
	if len(channels) > 0 {
		message += fmt.Sprintf("\n\n⚠️ Note: %d channel(s) were subscribed to this repository and have been unsubscribed.", len(channels))
	}

	h.followUpSuccess(s, i, message)
}

// handleListRepos handles the /list-repos command
func (h *CommandHandler) handleListRepos(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[LIST-REPOS] Command triggered by user %s", i.Member.User.ID)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[LIST-REPOS] ERROR: Failed to send deferred response: %v", err)
		return
	}

	repos, err := h.githubRepo.GetAllRepositories()
	if err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to retrieve repositories: %v", err))
		return
	}

	if len(repos) == 0 {
		h.followUpSuccess(s, i, "📦 No GitHub repositories registered yet.\n\nUse `/register-repo` to add one!")
		return
	}

	var response strings.Builder
	response.WriteString(fmt.Sprintf("📦 **Registered GitHub Repositories** (%d)\n\n", len(repos)))

	for _, repo := range repos {
		channels, _ := h.githubRepo.GetRepoChannels(repo.ID)
		pendingCount, _ := h.githubRepo.GetPendingCount(repo.ID)
		lastChecked, _ := h.githubRepo.GetLastChecked(repo.ID)

		response.WriteString(fmt.Sprintf("**%s** (`%s/%s`)\n", repo.ID, repo.Owner, repo.Name))
		response.WriteString(fmt.Sprintf("  🌿 Branch: `%s`\n", repo.TargetBranch))
		response.WriteString(fmt.Sprintf("  📢 Channels: %d\n", len(channels)))
		response.WriteString(fmt.Sprintf("  ⏳ Pending PRs: %d\n", pendingCount))
		if !lastChecked.IsZero() {
			response.WriteString(fmt.Sprintf("  🕒 Last checked: <t:%d:R>\n", lastChecked.Unix()))
		}
		response.WriteString("\n")
	}

	h.followUpSuccess(s, i, response.String())
}

// handleSetupRepoChannel handles the /setup-repo-channel command
func (h *CommandHandler) handleSetupRepoChannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[SETUP-REPO-CHANNEL] Command triggered by user %s", i.Member.User.ID)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[SETUP-REPO-CHANNEL] ERROR: Failed to send deferred response: %v", err)
		return
	}

	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.followUpError(s, i, "❌ Missing required parameters.")
		return
	}

	channelValue := options[0].ChannelValue(s)
	if channelValue == nil {
		h.followUpError(s, i, "❌ Invalid channel.")
		return
	}

	// Verify channel is in the same guild
	if channelValue.GuildID != i.GuildID {
		h.followUpError(s, i, "❌ Channel must be in this server.")
		return
	}

	// Verify it's a text channel
	if channelValue.Type != discordgo.ChannelTypeGuildText {
		h.followUpError(s, i, "❌ Please select a text channel.")
		return
	}

	channelID := channelValue.ID
	repoID := options[1].StringValue()

	// Check if repository exists
	exists, err := h.githubRepo.HasRepository(repoID)
	if err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Error checking repository: %v", err))
		return
	}
	if !exists {
		h.followUpError(s, i, fmt.Sprintf("❌ Repository `%s` not found. Use `/register-repo` first.", repoID))
		return
	}

	// Get repository details
	repo, err := h.githubRepo.GetRepository(repoID)
	if err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to get repository details: %v", err))
		return
	}

	// Add channel to repository
	if err := h.githubRepo.AddRepoChannel(repoID, channelID); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to setup channel: %v", err))
		return
	}

	// Check if there are pending PRs to process now that a channel is registered
	pendingCount, err := h.githubRepo.GetPendingCount(repoID)
	if err == nil && pendingCount > 0 {
		log.Printf("[SETUP-REPO-CHANNEL] Found %d pending PRs for %s, triggering immediate processing", pendingCount, repoID)
		if h.githubMonitor != nil {
			go h.githubMonitor.ProcessPendingPRsNow(repoID)
		}
	}

	message := fmt.Sprintf("✅ **Channel Configured**\n"+
		"📢 <#%s> will now receive PR summaries from:\n"+
		"📦 **%s** (`%s/%s`)\n\n",
		channelID, repo.ID, repo.Owner, repo.Name)

	if pendingCount > 0 {
		message += fmt.Sprintf("📬 Processing %d pending PRs...", pendingCount)
	}

	h.followUpSuccess(s, i, message)
}

// handleRemoveRepoChannel handles the /remove-repo-channel command
func (h *CommandHandler) handleRemoveRepoChannel(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[REMOVE-REPO-CHANNEL] Command triggered by user %s", i.Member.User.ID)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[REMOVE-REPO-CHANNEL] ERROR: Failed to send deferred response: %v", err)
		return
	}

	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.followUpError(s, i, "❌ Missing required parameters.")
		return
	}

	channelID := options[0].ChannelValue(s).ID
	repoID := options[1].StringValue()

	// Remove channel from repository
	if err := h.githubRepo.RemoveRepoChannel(repoID, channelID); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to remove channel: %v", err))
		return
	}

	h.followUpSuccess(s, i, fmt.Sprintf("✅ <#%s> will no longer receive PR summaries from `%s`.", channelID, repoID))
}

// handleScheduleRepo handles the /schedule-repo command
func (h *CommandHandler) handleScheduleRepo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[SCHEDULE-REPO] Command triggered by user %s", i.Member.User.ID)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[SCHEDULE-REPO] ERROR: Failed to send deferred response: %v", err)
		return
	}

	if !h.hasManageServerPermission(i.Member) {
		h.followUpError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.followUpError(s, i, "❌ Missing required parameters.")
		return
	}

	repoID := options[0].StringValue()
	timesStr := options[1].StringValue()

	// Check if repository exists
	exists, err := h.githubRepo.HasRepository(repoID)
	if err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Error checking repository: %v", err))
		return
	}
	if !exists {
		h.followUpError(s, i, fmt.Sprintf("❌ Repository `%s` not found. Use `/register-repo` first.", repoID))
		return
	}

	// Parse times
	var times []string
	if timesStr != "" {
		times = strings.Split(timesStr, ",")
		for i := range times {
			times[i] = strings.TrimSpace(times[i])
		}
	}

	// Validate schedule times
	if err := validateScheduleTimes(times); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	// Set schedule
	if err := h.githubRepo.SetSchedule(repoID, times); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ Failed to set schedule: %v", err))
		return
	}

	if len(times) > 0 {
		h.followUpSuccess(s, i, fmt.Sprintf("✅ **Schedule Updated**\n"+
			"📦 Repository: `%s`\n"+
			"⏰ Check times: %s\n\n"+
			"The bot will check for new PRs at these times daily.",
			repoID, strings.Join(times, ", ")))
	} else {
		h.followUpSuccess(s, i, fmt.Sprintf("✅ **Schedule Cleared**\n"+
			"📦 Repository: `%s`\n"+
			"The bot will use the default check interval (%s minutes).",
			repoID, os.Getenv("GITHUB_CHECK_INTERVAL_MINUTES")))
	}
}

// handleUpdateRepo handles the /update-repo command
func (h *CommandHandler) handleUpdateRepo(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[UPDATE-REPO] Command triggered by user %s", i.Member.User.ID)

	// Check permissions first
	if !h.hasManageServerPermission(i.Member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Check rate limit
	if limited, remaining := updateRepoRateLimiter.Check(i.Member.User.ID); limited {
		h.respondError(s, i, fmt.Sprintf("⏳ Please wait %d seconds before triggering another update.", int(remaining.Seconds())))
		return
	}

	// Get repository ID
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondError(s, i, "❌ You need to specify a repository ID.")
		return
	}
	repoID := options[0].StringValue()

	// Verify repository exists
	exists, err := h.githubRepo.HasRepository(repoID)
	if err != nil {
		h.respondError(s, i, fmt.Sprintf("❌ Error checking repository: %v", err))
		return
	}
	if !exists {
		h.respondError(s, i, fmt.Sprintf("❌ Repository `%s` not found.\n\nUse `/list-repos` to see available repositories.", repoID))
		return
	}

	// Get repository details
	repo, err := h.githubRepo.GetRepository(repoID)
	if err != nil {
		h.respondError(s, i, fmt.Sprintf("❌ Failed to get repository details: %v", err))
		return
	}

	// Check if GitHub monitor is available
	if h.githubMonitor == nil {
		h.respondError(s, i, "❌ GitHub monitoring is not enabled.")
		return
	}

	// Send initial response
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔄 Checking for updates from **%s/%s**...", repo.Owner, repo.Name),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("[UPDATE-REPO] ERROR: Failed to send initial response: %v", err)
		return
	}

	// Trigger immediate check for specific repository
	updateRepoRateLimiter.Record(i.Member.User.ID) // Record the command use
	
	go func() {
		h.githubMonitor.CheckRepositoryNow(repoID)

		// Send follow-up message
		followupMessage := fmt.Sprintf("✅ Check for **%s/%s** completed! If there are any updates, they were posted to registered channels.", repo.Owner, repo.Name)
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: followupMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		if err != nil {
			log.Printf("[UPDATE-REPO] ERROR: Failed to send followup message: %v", err)
		}
	}()

	log.Printf("Manual repository update triggered for %s by user in guild %s", repoID, i.GuildID)
}

// handleUpdateAllRepos handles the /update-all-repos command
func (h *CommandHandler) handleUpdateAllRepos(s *discordgo.Session, i *discordgo.InteractionCreate) {
	log.Printf("[UPDATE-ALL-REPOS] Command triggered by user %s", i.Member.User.ID)

	// Check permissions first
	if !h.hasManageServerPermission(i.Member) {
		h.respondError(s, i, "❌ You need the **Manage Server** permission to use this command.")
		return
	}

	// Check rate limit
	if limited, remaining := updateRepoRateLimiter.Check(i.Member.User.ID); limited {
		h.respondError(s, i, fmt.Sprintf("⏳ Please wait %d seconds before triggering another update.", int(remaining.Seconds())))
		return
	}

	// Check if GitHub monitor is available
	if h.githubMonitor == nil {
		h.respondError(s, i, "❌ GitHub monitoring is not enabled.")
		return
	}

	// Get all repositories
	repos, err := h.githubRepo.GetAllRepositories()
	if err != nil {
		h.respondError(s, i, fmt.Sprintf("❌ Failed to retrieve repositories: %v", err))
		return
	}

	if len(repos) == 0 {
		h.respondError(s, i, "📦 No GitHub repositories registered yet.\n\nUse `/register-repo` to add one!")
		return
	}

	// Send initial response
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🔄 Checking all %d registered repositories for updates...", len(repos)),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("[UPDATE-ALL-REPOS] ERROR: Failed to send initial response: %v", err)
		return
	}

	// Trigger immediate check for all repositories
	updateRepoRateLimiter.Record(i.Member.User.ID) // Record the command use
	
	go func() {
		h.githubMonitor.CheckAllRepositoriesNow()

		// Send follow-up message
		followupMessage := fmt.Sprintf("✅ Check for all %d repositories completed! If there are any updates, they were posted to registered channels.", len(repos))
		_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: followupMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		if err != nil {
			log.Printf("[UPDATE-ALL-REPOS] ERROR: Failed to send followup message: %v", err)
		}
	}()

	log.Printf("Manual update triggered for all repositories by user in guild %s", i.GuildID)
}

