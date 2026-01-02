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
		}
	})
}

// handleSetupNews handles the /setup-news command
func (h *CommandHandler) handleSetupNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Não foi possível verificar suas permissões.")
		return
	}

	// Check if user has "Manage Server" permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão **Gerenciar Servidor** para usar este comando.")
		return
	}

	// Get the channel parameter
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondError(s, i, "❌ Você precisa especificar um canal.")
		return
	}

	// Extract channel ID from the option
	channelValue := options[0].ChannelValue(s)
	if channelValue == nil {
		h.respondError(s, i, "❌ Canal inválido.")
		return
	}
	
	channelID := channelValue.ID

	// Get feed identifier (default to godot-official)
	feedID := "godot-official"
	if len(options) > 1 {
		feedID = options[1].StringValue()
	}

	// Verify feed exists
	hasFeed, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "Erro ao verificar o feed.")
		return
	}
	if !hasFeed {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' não encontrado. Use /list-feeds para ver os feeds disponíveis.", feedID))
		return
	}

	// Verify it's a text channel
	if channelValue.Type != discordgo.ChannelTypeGuildText {
		h.respondError(s, i, "❌ Apenas canais de texto podem receber notícias.")
		return
	}

	// Check if channel is already subscribed to this feed
	feeds, err := h.channelRepo.GetChannelFeeds(channelID)
	if err != nil {
		log.Printf("Error checking channel feeds: %v", err)
		h.respondError(s, i, "Erro ao verificar o canal.")
		return
	}

	for _, f := range feeds {
		if f == feedID {
			h.respondError(s, i, fmt.Sprintf("⚠️ Este canal já está inscrito no feed '%s'.", feedID))
			return
		}
	}

	// Check if limit would be exceeded (count unique channels)
	count, err := h.channelRepo.GetChannelCount()
	if err != nil {
		log.Printf("Error getting channel count: %v", err)
		h.respondError(s, i, "Erro ao verificar o limite de canais.")
		return
	}

	if count >= h.maxLimit && len(feeds) == 0 {
		// Only enforce limit for new channels, not for adding feeds to existing channels
		h.respondError(s, i, fmt.Sprintf("❌ Limite de canais atingido (%d/%d). Não é possível adicionar mais canais.", count, h.maxLimit))
		return
	}

	// Add the channel-feed association
	if err := h.channelRepo.AddChannel(channelID, feedID); err != nil {
		log.Printf("Error adding channel: %v", err)
		h.respondError(s, i, "Erro ao registrar o canal.")
		return
	}

	// Get feed info for response
	feed, err := h.feedRepo.GetFeed(feedID)
	if err != nil {
		log.Printf("Error getting feed info: %v", err)
		// Continue anyway, feed exists
		feed = &storage.Feed{ID: feedID, Title: feedID}
	}

	// Success response
	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Canal configurado com sucesso!**\n\n<#%s> agora receberá notícias de **%s** (%s).",
		channelID,
		feed.Title,
		feedID,
	))

	log.Printf("Channel %s subscribed to feed %s in guild %s", channelID, feedID, i.GuildID)
}

// handleRemoveNews handles the /remove-news command
func (h *CommandHandler) handleRemoveNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Não foi possível verificar suas permissões.")
		return
	}

	// Check if user has "Manage Server" permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão **Gerenciar Servidor** para usar este comando.")
		return
	}

	// Get the channel parameter
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondError(s, i, "❌ Você precisa especificar um canal.")
		return
	}

	// Extract channel ID from the option
	channelValue := options[0].ChannelValue(s)
	if channelValue == nil {
		h.respondError(s, i, "❌ Canal inválido.")
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
		h.respondError(s, i, "Erro ao verificar o canal.")
		return
	}

	if len(feeds) == 0 {
		h.respondError(s, i, "⚠️ Este canal não está inscrito em nenhum feed.")
		return
	}

	// If no feed specified and channel has multiple feeds, show them
	if feedID == "" && len(feeds) > 1 {
		feedList := ""
		for _, f := range feeds {
			feedList += fmt.Sprintf("• %s\n", f)
		}
		h.respondError(s, i, fmt.Sprintf("⚠️ Este canal está inscrito em múltiplos feeds. Especifique qual remover:\n\n%s", feedList))
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
		h.respondError(s, i, fmt.Sprintf("⚠️ Este canal não está inscrito no feed '%s'.", feedID))
		return
	}

	// Remove the channel-feed association
	if err := h.channelRepo.RemoveChannel(channelID, feedID); err != nil {
		log.Printf("Error removing channel: %v", err)
		h.respondError(s, i, "Erro ao remover o canal.")
		return
	}

	// Success response
	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Canal removido com sucesso!**\n\n<#%s> não receberá mais notícias de **%s**.",
		channelID,
		feedID,
	))

	log.Printf("Channel %s unsubscribed from feed %s in guild %s", channelID, feedID, i.GuildID)
}

// handleListChannels handles the /list-channels command
func (h *CommandHandler) handleListChannels(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Não foi possível verificar suas permissões.")
		return
	}

	// Check if user has "Manage Server" permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão **Gerenciar Servidor** para usar este comando.")
		return
	}

	// Get all channels
	channels, err := h.channelRepo.GetAllChannels()
	if err != nil {
		log.Printf("Error getting channels: %v", err)
		h.respondError(s, i, "Erro ao buscar os canais.")
		return
	}

	if len(channels) == 0 {
		h.respondSuccess(s, i, "📋 **Nenhum canal registrado**\n\nUse `/setup-news` em um canal para começar a receber notícias.")
		return
	}

	// Build channel list
	var response string
	response = fmt.Sprintf("📋 **Canais Registrados** (%d/%d)\n\n", len(channels), h.maxLimit)

	for i, channelID := range channels {
		// Use simple channel mention format
		response += fmt.Sprintf("%d. <#%s>\n", i+1, channelID)
	}

	response += fmt.Sprintf("\n💡 Use `/remove-news` em um canal para removê-lo da lista.")

	h.respondSuccess(s, i, response)

	log.Printf("Channel list requested by user in guild %s", i.GuildID)
}

// handleUpdateNews handles the /update-news command (specific feed)
func (h *CommandHandler) handleUpdateNews(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if command was used in a guild (server)
	if i.GuildID == "" {
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Não foi possível verificar suas permissões.")
		return
	}

	// Check for Manage Server permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão 'Gerenciar Servidor' para usar este comando.")
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
		h.respondError(s, i, "❌ Erro ao verificar feed.")
		return
	}
	if !hasFeed {
		h.respondError(s, i, fmt.Sprintf("❌ Feed não encontrado: %s\n\nUse `/list-feeds` para ver os feeds disponíveis.", feedID))
		return
	}

	// Check if feed has any channels subscribed
	channels, err := h.channelRepo.GetFeedChannels(feedID)
	if err != nil {
		log.Printf("Error getting feed channels: %v", err)
		h.respondError(s, i, "❌ Erro ao verificar canais registrados.")
		return
	}

	if len(channels) == 0 {
		h.respondError(s, i, fmt.Sprintf("⚠️ Nenhum canal registrado para o feed: %s\n\nUse `/setup-news` para configurar um canal primeiro.", feedID))
		return
	}

	// Check if bot is set
	if h.bot == nil {
		h.respondError(s, i, "❌ Bot não está configurado corretamente.")
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
			Content: fmt.Sprintf("🔄 Verificando novidades de **%s**...", feed.Title),
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
			followupMessage = fmt.Sprintf("✅ Verificação de **%s** concluída! Se houver novidades, elas foram publicadas nos canais registrados.", feed.Title)
		} else {
			followupMessage = fmt.Sprintf("ℹ️ Nenhuma novidade encontrada em **%s** ou houve um erro. Verifique os logs para mais detalhes.", feed.Title)
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
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Get the member who executed the command
	member := i.Member
	if member == nil {
		h.respondError(s, i, "Não foi possível verificar suas permissões.")
		return
	}

	// Check for Manage Server permission
	if !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão 'Gerenciar Servidor' para usar este comando.")
		return
	}

	// Check if there are any channels registered
	count, err := h.channelRepo.GetChannelCount()
	if err != nil {
		log.Printf("Error getting channel count: %v", err)
		h.respondError(s, i, "❌ Erro ao verificar canais registrados.")
		return
	}

	if count == 0 {
		h.respondError(s, i, "⚠️ Nenhum canal registrado para receber notícias.\n\nUse `/setup-news` para configurar um canal primeiro.")
		return
	}

	// Check if bot is set
	if h.bot == nil {
		h.respondError(s, i, "❌ Bot não está configurado corretamente.")
		return
	}

	// Send initial response
	respondErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🔄 Verificando novidades de todos os feeds...",
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
			followupMessage = "✅ Verificação de todos os feeds concluída! Se houver novidades, elas foram publicadas nos canais registrados."
		} else {
			followupMessage = "ℹ️ Nenhuma novidade encontrada ou houve um erro. Verifique os logs para mais detalhes."
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
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Check permissions
	member := i.Member
	if member == nil || !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão **Gerenciar Servidor** para usar este comando.")
		return
	}

	// Get parameters
	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.respondError(s, i, "❌ Você precisa especificar identifier e url.")
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
		h.respondError(s, i, "Erro ao verificar o feed.")
		return
	}

	if exists {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' já está registrado.", feedID))
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
		h.respondError(s, i, "Erro ao registrar o feed.")
		return
	}

	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Feed registrado com sucesso!**\n\n**ID:** %s\n**Título:** %s\n**URL:** %s",
		feedID, title, feedURL,
	))

	log.Printf("Feed %s registered in guild %s", feedID, i.GuildID)
}

// handleUnregisterFeed handles the /unregister-feed command
func (h *CommandHandler) handleUnregisterFeed(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check guild
	if i.GuildID == "" {
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Check permissions
	member := i.Member
	if member == nil || !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão **Gerenciar Servidor** para usar este comando.")
		return
	}

	// Get parameter
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondError(s, i, "❌ Você precisa especificar o identifier do feed.")
		return
	}

	feedID := options[0].StringValue()

	// Check if feed exists
	exists, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "Erro ao verificar o feed.")
		return
	}

	if !exists {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' não encontrado.", feedID))
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
		h.respondError(s, i, "Erro ao remover o feed.")
		return
	}

	// If channels were using this feed, remove associations
	if len(channels) > 0 {
		for _, chID := range channels {
			_ = h.channelRepo.RemoveChannel(chID, feedID)
		}
	}

	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Feed removido com sucesso!**\n\nFeed '%s' foi removido%s.",
		feedID,
		func() string {
			if len(channels) > 0 {
				return fmt.Sprintf(" e desvinculado de %d canal(is)", len(channels))
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
		h.respondError(s, i, "Erro ao listar feeds.")
		return
	}

	if len(feeds) == 0 {
		h.respondError(s, i, "ℹ️ Nenhum feed registrado.")
		return
	}

	// Build response
	response := "📰 **Feeds Registrados**\n\n"
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
			response += fmt.Sprintf("└ Horários: %s\n", times)
		}
		
		// Show channel count
		channels, err := h.channelRepo.GetFeedChannels(feed.ID)
		if err == nil {
			response += fmt.Sprintf("└ Canais inscritos: %d\n", len(channels))
		}
		response += "\n"
	}

	h.respondSuccess(s, i, response)
}

// handleScheduleFeed handles the /schedule-feed command
func (h *CommandHandler) handleScheduleFeed(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check guild
	if i.GuildID == "" {
		h.respondError(s, i, "Este comando só pode ser usado em um servidor.")
		return
	}

	// Check permissions
	member := i.Member
	if member == nil || !h.hasManageServerPermission(member) {
		h.respondError(s, i, "❌ Você precisa da permissão **Gerenciar Servidor** para usar este comando.")
		return
	}

	// Get parameters
	options := i.ApplicationCommandData().Options
	if len(options) < 2 {
		h.respondError(s, i, "❌ Você precisa especificar identifier e times.")
		return
	}

	feedID := options[0].StringValue()
	timesStr := options[1].StringValue()

	// Check if feed exists
	exists, err := h.feedRepo.HasFeed(feedID)
	if err != nil {
		log.Printf("Error checking feed: %v", err)
		h.respondError(s, i, "Erro ao verificar o feed.")
		return
	}

	if !exists {
		h.respondError(s, i, fmt.Sprintf("❌ Feed '%s' não encontrado.", feedID))
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
		h.respondError(s, i, fmt.Sprintf("❌ Erro ao configurar horários: %v", err))
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
		"✅ **Horários configurados!**\n\nFeed '%s' será verificado nos seguintes horários:\n%s",
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
