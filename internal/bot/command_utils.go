package bot

import (
"log"
"time"

"github.com/bwmarrin/discordgo"
)

// Command Utility Functions
// Shared helper functions used across all command handlers

// Rate limiters for manual update commands (30 second cooldown)
var (
	updateNewsRateLimiter    = NewRateLimiter(30 * time.Second)
	updateRepoRateLimiter    = NewRateLimiter(30 * time.Second)
)
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
	// Truncate message to Discord's 2000 character limit
	message = truncateMessage(message, 2000)
	
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
	// Truncate message to Discord's 2000 character limit
	message = truncateMessage(message, 2000)
	
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
func (h *CommandHandler) followUpError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	// Truncate message to Discord's 2000 character limit
	message = truncateMessage(message, 2000)
	
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
	// Truncate message to Discord's 2000 character limit
	message = truncateMessage(message, 2000)
	
	_, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: message,
	})
	if err != nil {
		log.Printf("Error sending follow-up success: %v", err)
	}
}
