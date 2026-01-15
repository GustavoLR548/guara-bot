package bot

import (
"fmt"
"log"

"github.com/bwmarrin/discordgo"
)

// Language Configuration Commands
// This file contains language setting command handlers
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

	// Validate language code
	if err := isValidLanguageCode(languageCode); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

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

	// Verify channel is in the same guild
	if channelValue.GuildID != i.GuildID {
		h.followUpError(s, i, "❌ Channel must be in this server.")
		return
	}

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

	// Validate language code
	if err := isValidLanguageCode(languageCode); err != nil {
		h.followUpError(s, i, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

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
