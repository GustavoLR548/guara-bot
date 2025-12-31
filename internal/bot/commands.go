package bot

import (
	"fmt"
	"log"

	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// CommandHandler handles Discord slash commands
type CommandHandler struct {
	channelRepo storage.ChannelRepository
	maxLimit    int
	bot         *Bot // Reference to bot for triggering updates
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(channelRepo storage.ChannelRepository, maxLimit int) *CommandHandler {
	return &CommandHandler{
		channelRepo: channelRepo,
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
			Description: "Configure a channel to receive Godot news updates",
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
			},
		},
		{
			Name:        "remove-news",
			Description: "Remove a channel from receiving Godot news updates",
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
			},
		},
		{
			Name:        "list-channels",
			Description: "List all channels registered for news updates (Admin only)",
		},
		{
			Name:        "update-news",
			Description: "Force an immediate check for new Godot news (Admin only)",
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

	// Verify it's a text channel
	if channelValue.Type != discordgo.ChannelTypeGuildText {
		h.respondError(s, i, "❌ Apenas canais de texto podem receber notícias.")
		return
	}

	// Check if channel is already registered
	hasChannel, err := h.channelRepo.HasChannel(channelID)
	if err != nil {
		log.Printf("Error checking channel: %v", err)
		h.respondError(s, i, "Erro ao verificar o canal.")
		return
	}

	if hasChannel {
		h.respondError(s, i, "⚠️ Este canal já está registrado para receber notícias.")
		return
	}

	// Check if limit would be exceeded
	count, err := h.channelRepo.GetChannelCount()
	if err != nil {
		log.Printf("Error getting channel count: %v", err)
		h.respondError(s, i, "Erro ao verificar o limite de canais.")
		return
	}

	if count >= h.maxLimit {
		h.respondError(s, i, fmt.Sprintf("❌ Limite de canais atingido (%d/%d). Não é possível adicionar mais canais.", count, h.maxLimit))
		return
	}

	// Add the channel
	if err := h.channelRepo.AddChannel(channelID); err != nil {
		log.Printf("Error adding channel: %v", err)
		h.respondError(s, i, "Erro ao registrar o canal.")
		return
	}

	// Success response
	newCount := count + 1
	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Canal configurado com sucesso!**\n\n<#%s> agora receberá notícias do Godot Engine a cada 15 minutos.\n\n📊 Canais registrados: %d/%d",
		channelID,
		newCount,
		h.maxLimit,
	))

	log.Printf("Channel %s registered successfully in guild %s", channelID, i.GuildID)
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

	// Check if channel is registered
	hasChannel, err := h.channelRepo.HasChannel(channelID)
	if err != nil {
		log.Printf("Error checking channel: %v", err)
		h.respondError(s, i, "Erro ao verificar o canal.")
		return
	}

	if !hasChannel {
		h.respondError(s, i, "⚠️ Este canal não está registrado para receber notícias.")
		return
	}

	// Remove the channel
	if err := h.channelRepo.RemoveChannel(channelID); err != nil {
		log.Printf("Error removing channel: %v", err)
		h.respondError(s, i, "Erro ao remover o canal.")
		return
	}

	// Get updated count
	count, _ := h.channelRepo.GetChannelCount()

	// Success response
	h.respondSuccess(s, i, fmt.Sprintf(
		"✅ **Canal removido com sucesso!**\n\n<#%s> não receberá mais notícias do Godot Engine.\n\n📊 Canais registrados: %d/%d",
		channelID,
		count,
		h.maxLimit,
	))

	log.Printf("Channel %s removed successfully from guild %s", channelID, i.GuildID)
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

// handleUpdateNews handles the /update-news command
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
			Content: "🔄 Verificando novidades do Godot...",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if respondErr != nil {
		log.Printf("Error sending initial response: %v", respondErr)
		return
	}

	// Trigger immediate news check
	go func() {
		result := h.bot.CheckAndPostNews()
		
		var followupMessage string
		if result {
			followupMessage = "✅ Verificação concluída! Se houver novidades, elas foram publicadas nos canais registrados."
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

	log.Printf("Manual news update triggered by user in guild %s", i.GuildID)
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
