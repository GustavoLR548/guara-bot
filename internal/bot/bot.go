package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ai"
	"github.com/GustavoLR548/godot-news-bot/internal/news"
	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// Bot represents the Discord bot with its dependencies
type Bot struct {
	session      *discordgo.Session
	newsFetcher  news.NewsFetcher
	aiSummarizer ai.AISummarizer
	channelRepo  storage.ChannelRepository
	historyRepo  storage.HistoryRepository
	checkInterval time.Duration
	stopChan     chan bool
}

// NewBot creates a new bot instance with all dependencies
func NewBot(
	session *discordgo.Session,
	newsFetcher news.NewsFetcher,
	aiSummarizer ai.AISummarizer,
	channelRepo storage.ChannelRepository,
	historyRepo storage.HistoryRepository,
	checkInterval time.Duration,
) *Bot {
	return &Bot{
		session:       session,
		newsFetcher:   newsFetcher,
		aiSummarizer:  aiSummarizer,
		channelRepo:   channelRepo,
		historyRepo:   historyRepo,
		checkInterval: checkInterval,
		stopChan:      make(chan bool),
	}
}

// Start begins the news checking loop
func (b *Bot) Start() {
	log.Println("Starting news check loop...")
	
	// Run immediately on start
	b.checkAndPostNews()
	
	ticker := time.NewTicker(b.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.checkAndPostNews()
		case <-b.stopChan:
			log.Println("News loop stopped")
			return
		}
	}
}

// Stop gracefully stops the news checking loop
func (b *Bot) Stop() {
	close(b.stopChan)
}

// CheckAndPostNews checks for new articles and posts them (public method for manual triggers)
// Returns true if successful (news found and posted or no news), false if error occurred
func (b *Bot) CheckAndPostNews() bool {
	log.Println("Manual check for new articles triggered...")
	b.checkAndPostNews()
	return true
}

// checkAndPostNews checks for new articles and posts them
func (b *Bot) checkAndPostNews() {
	log.Println("Checking for new articles...")

	// Increase timeout to 5 minutes to ensure Gemini has enough time
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// First, check if there are any channels registered
	channels, err := b.channelRepo.GetAllChannels()
	if err != nil {
		log.Printf("Error getting channels: %v", err)
		return
	}

	// If channels exist, process pending queue first
	if len(channels) > 0 {
		pendingGUIDs, err := b.historyRepo.GetPending()
		if err != nil {
			log.Printf("Error getting pending queue: %v", err)
		} else if len(pendingGUIDs) > 0 {
			log.Printf("Processing %d pending article(s)...", len(pendingGUIDs))
			for _, guid := range pendingGUIDs {
				// Try to fetch and post this pending article
				if err := b.processPendingArticle(ctx, guid, channels); err != nil {
					log.Printf("Error processing pending article %s: %v", guid, err)
				} else {
					// Successfully posted, remove from pending
					if err := b.historyRepo.RemoveFromPending(guid); err != nil {
						log.Printf("Error removing from pending: %v", err)
					}
				}
			}
		}
	}

	// Now check for new articles
	article, err := b.newsFetcher.FetchLatestArticle()
	if err != nil {
		log.Printf("Error fetching article: %v", err)
		return
	}

	// Check if article is new
	lastGUID, err := b.historyRepo.GetLastGUID()
	if err != nil {
		log.Printf("Error getting last GUID: %v", err)
		return
	}

	if !news.IsNewArticle(article.GUID, lastGUID) {
		log.Println("No new articles found")
		return
	}

	log.Printf("New article found: %s", article.Title)

	// If no channels registered, add to pending queue
	if len(channels) == 0 {
		log.Println("No channels registered to post news, adding to pending queue")
		if err := b.historyRepo.AddToPending(article.GUID); err != nil {
			log.Printf("Error adding to pending queue: %v", err)
		} else {
			log.Printf("Article added to pending queue (will be posted when channels are registered)")
		}
		// Update last GUID so we don't keep detecting this as "new"
		if err := b.historyRepo.SaveGUID(article.GUID); err != nil {
			log.Printf("Error saving GUID: %v", err)
		}
		return
	}

	// Channels exist, process the article normally
	if err := b.processAndPostArticle(ctx, article, channels); err != nil {
		log.Printf("Error processing article: %v", err)
	}
}

// processPendingArticle fetches and posts a pending article by GUID
func (b *Bot) processPendingArticle(ctx context.Context, guid string, channels []string) error {
	// Fetch the full feed to find this article
	articles, err := b.newsFetcher.FetchArticles()
	if err != nil {
		return fmt.Errorf("failed to fetch articles: %w", err)
	}

	// Find the article with this GUID
	var article *news.Article
	for _, a := range articles {
		if a.GUID == guid {
			article = &a
			break
		}
	}

	if article == nil {
		log.Printf("Pending article with GUID %s not found in feed (may be too old)", guid)
		return nil // Don't return error, just skip it
	}

	log.Printf("Processing pending article: %s", article.Title)
	return b.processAndPostArticle(ctx, article, channels)
}

// processAndPostArticle scrapes, summarizes, and posts an article
func (b *Bot) processAndPostArticle(ctx context.Context, article *news.Article, channels []string) error {
	log.Printf("Generating summary for %d registered channel(s)...", len(channels))

	// Scrape article content
	content, err := b.newsFetcher.ScrapeArticleContent(article.Link)
	if err != nil {
		return fmt.Errorf("failed to scrape content: %w", err)
	}

	// Generate summary
	summary, err := b.aiSummarizer.Summarize(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	log.Printf("Summary generated for: %s", article.Title)

	// Create embed message
	embed := b.createNewsEmbed(article, summary)

	// Broadcast to all channels
	successCount := 0
	for _, channelID := range channels {
		if err := b.sendEmbed(channelID, embed); err != nil {
			log.Printf("Error sending to channel %s: %v", channelID, err)
		} else {
			successCount++
		}
	}

	log.Printf("Article posted to %d/%d channels", successCount, len(channels))

	// Save GUID to history
	if err := b.historyRepo.SaveGUID(article.GUID); err != nil {
		return fmt.Errorf("failed to save GUID: %w", err)
	}

	return nil
}

// createNewsEmbed creates a Discord embed for the news article
func (b *Bot) createNewsEmbed(article *news.Article, summary string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       article.Title,
		Description: summary,
		URL:         article.Link,
		Color:       0x478CBF, // Godot blue color
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Godot Engine News",
		},
		Timestamp: article.PublishDate.Format(time.RFC3339),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "🔗 Link Completo",
				Value:  fmt.Sprintf("[Ler artigo completo](%s)", article.Link),
				Inline: false,
			},
		},
	}
}

// sendEmbed sends an embed message to a specific channel
func (b *Bot) sendEmbed(channelID string, embed *discordgo.MessageEmbed) error {
	_, err := b.session.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return fmt.Errorf("failed to send message to channel %s: %w", channelID, err)
	}
	return nil
}

// BroadcastMessage sends a message to all registered channels (useful for testing/announcements)
func (b *Bot) BroadcastMessage(message string) error {
	channels, err := b.channelRepo.GetAllChannels()
	if err != nil {
		return fmt.Errorf("failed to get channels: %w", err)
	}

	if len(channels) == 0 {
		return fmt.Errorf("no channels registered")
	}

	for _, channelID := range channels {
		_, err := b.session.ChannelMessageSend(channelID, message)
		if err != nil {
			log.Printf("Failed to send to channel %s: %v", channelID, err)
		}
	}

	return nil
}
