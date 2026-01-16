package bot

import (
	"fmt"
	"log"

	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// CommandHandler handles Discord slash commands
// Individual command implementations are split across:
//   - rss_commands.go: RSS feed management commands
//   - github_commands.go: GitHub repository commands
//   - language_commands.go: Language configuration commands
//   - command_utils.go: Shared utility functions
type CommandHandler struct {
	channelRepo   storage.ChannelRepository
	feedRepo      storage.RSSFeedRepository
	githubRepo    storage.GitHubRepository
	maxLimit      int
	bot           *Bot           // Reference to bot for triggering updates
	githubMonitor *GitHubMonitor // Reference to GitHub monitor for triggering updates
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(channelRepo storage.ChannelRepository, feedRepo storage.RSSFeedRepository, githubRepo storage.GitHubRepository, maxLimit int) *CommandHandler {
	return &CommandHandler{
		channelRepo: channelRepo,
		feedRepo:    feedRepo,
		githubRepo:  githubRepo,
		maxLimit:    maxLimit,
}
}

// SetBot sets the bot reference (called after bot is created)
func (h *CommandHandler) SetBot(bot *Bot) {
	h.bot = bot
}

// SetGitHubMonitor sets the GitHub monitor reference (called after monitor is created)
func (h *CommandHandler) SetGitHubMonitor(monitor *GitHubMonitor) {
	h.githubMonitor = monitor
}

// RegisterCommands registers all slash commands with Discord
func (h *CommandHandler) RegisterCommands(s *discordgo.Session) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "setup-feed-channel",
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
			Name:        "remove-feed-channel",
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
		// GitHub Repository Commands
		{
			Name:        "register-repo",
			Description: "Register a GitHub repository to monitor for PR updates",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Unique identifier for this repository (e.g., 'godot-engine')",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "owner",
					Description: "Repository owner (e.g., 'godotengine')",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "repo",
					Description: "Repository name (e.g., 'godot')",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "branch",
					Description: "Target branch to monitor (default: 'main')",
					Required:    false,
				},
			},
		},
		{
			Name:        "unregister-repo",
			Description: "Remove a GitHub repository from monitoring",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Repository identifier",
					Required:    true,
				},
			},
		},
		{
			Name:        "list-repos",
			Description: "List all registered GitHub repositories",
		},
		{
			Name:        "setup-repo-channel",
			Description: "Configure a channel to receive PR summaries from a repository",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "The channel to setup for PR updates",
					Required:    true,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "repo",
					Description: "Repository identifier",
					Required:    true,
				},
			},
		},
		{
			Name:        "remove-repo-channel",
			Description: "Remove a channel from receiving PR summaries",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "The channel to remove",
					Required:    true,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "repo",
					Description: "Repository identifier",
					Required:    true,
				},
			},
		},
		{
			Name:        "schedule-repo",
			Description: "Set check times for a GitHub repository (e.g., 09:00,13:00,18:00)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "repo",
					Description: "Repository identifier",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "times",
					Description: "Comma-separated check times in HH:MM format (empty to use interval)",
					Required:    true,
				},
			},
		},
		{
			Name:        "update-repo",
			Description: "Force an immediate check for a specific GitHub repository",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "repo",
					Description: "Repository identifier",
					Required:    true,
				},
			},
		},
		{
			Name:        "update-all-repos",
			Description: "Force an immediate check for all registered GitHub repositories",
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

// HandleCommands sets up the command handler routing
func (h *CommandHandler) HandleCommands(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.ApplicationCommandData().Name {
		// RSS Feed Commands (rss_commands.go)
		case "setup-feed-channel":
			h.handleSetupNews(s, i)
		case "remove-feed-channel":
			h.handleRemoveNews(s, i)
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
			
		// Language Commands (language_commands.go)
		case "set-language":
			h.handleSetLanguage(s, i)
		case "set-channel-language":
			h.handleSetChannelLanguage(s, i)
			
		// View/Info Commands (commands.go)
		case "list-channels":
			h.handleListChannels(s, i)
		case "help":
			h.handleHelp(s, i)
			
		// GitHub Repository Commands (github_commands.go)
		case "register-repo":
			h.handleRegisterRepo(s, i)
		case "unregister-repo":
			h.handleUnregisterRepo(s, i)
		case "list-repos":
			h.handleListRepos(s, i)
		case "setup-repo-channel":
			h.handleSetupRepoChannel(s, i)
		case "remove-repo-channel":
			h.handleRemoveRepoChannel(s, i)
		case "schedule-repo":
			h.handleScheduleRepo(s, i)
		case "update-repo":
			h.handleUpdateRepo(s, i)
		case "update-all-repos":
			h.handleUpdateAllRepos(s, i)
		}
	})
}

// handleHelp handles the help command
func (h *CommandHandler) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	helpMessage := "🤖 **Bot Commands Help**\n\n" +
		"**RSS Feed Commands:**\n" +
		"• `/setup-feed-channel <channel> [feed]` - Subscribe a channel to RSS feed updates\n" +
		"• `/remove-feed-channel <channel> [feed]` - Unsubscribe a channel from RSS feed updates\n" +
		"• `/register-feed <feed-url>` - Register a new RSS feed\n" +
		"• `/unregister-feed <feed-url>` - Unregister an existing RSS feed\n" +
		"• `/list-feeds` - List all registered RSS feeds\n" +
		"• `/schedule-feed <feed-url> <interval-minutes>` - Schedule automatic updates for a feed\n" +
		"• `/update-news <channel>` - Manually trigger feed update for a channel\n" +
		"• `/update-all-news` - Manually trigger update for all channels\n\n" +
		"**GitHub Repository Commands:**\n" +
		"• `/register-repo <repo-url>` - Register a GitHub repository for monitoring\n" +
		"• `/unregister-repo <repo-url>` - Unregister a GitHub repository\n" +
		"• `/list-repos` - List all registered GitHub repositories\n" +
		"• `/setup-repo-channel <repo-url> <channel>` - Setup a channel for repository updates\n" +
		"• `/remove-repo-channel <repo-url> <channel>` - Remove a channel from repository updates\n" +
		"• `/schedule-repo <repo-url> <interval-minutes>` - Schedule automatic updates for a repository\n" +
		"• `/update-repo <repo-url>` - Manually trigger update for a specific repository\n" +
		"• `/update-all-repos` - Manually trigger update for all repositories\n\n" +
		"**Language Commands:**\n" +
		"• `/set-language <language>` - Set the server's default language\n" +
		"• `/set-channel-language <channel> <language>` - Set a channel's language\n\n" +
		"**Other Commands:**\n" +
		"• `/list-channels` - List all registered channels and their feeds/repos\n" +
		"• `/help` - Show this help message"

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: helpMessage,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Error responding to help command: %v", err)
	}
}
