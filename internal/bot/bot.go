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
	session       *discordgo.Session
	newsFetcher   news.NewsFetcher
	aiSummarizer  ai.AISummarizer
	channelRepo   storage.ChannelRepository
	historyRepo   storage.HistoryRepository
	feedRepo      storage.FeedRepository
	checkInterval time.Duration
	stopChan      chan bool
}

// NewBot creates a new bot instance with all dependencies
func NewBot(
	session *discordgo.Session,
	newsFetcher news.NewsFetcher,
	aiSummarizer ai.AISummarizer,
	channelRepo storage.ChannelRepository,
	historyRepo storage.HistoryRepository,
	feedRepo storage.FeedRepository,
	checkInterval time.Duration,
) *Bot {
	return &Bot{
		session:       session,
		newsFetcher:   newsFetcher,
		aiSummarizer:  aiSummarizer,
		channelRepo:   channelRepo,
		historyRepo:   historyRepo,
		feedRepo:      feedRepo,
		checkInterval: checkInterval,
		stopChan:      make(chan bool),
	}
}

// Start begins the news checking loop with time-based scheduling
func (b *Bot) Start() {
	log.Println("Starting multi-feed news check loop...")
	
	// Run immediately on start
	b.checkAndPostNews()
	
	// Check every minute for scheduled times
	ticker := time.NewTicker(1 * time.Minute)
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
	log.Println("Manual check for new articles triggered (all feeds)...")
	b.checkAndPostNews()
	return true
}

// CheckAndPostFeedNews checks for new articles from a specific feed (public method for manual triggers)
// Returns true if successful (news found and posted or no news), false if error occurred
func (b *Bot) CheckAndPostFeedNews(feedID string) bool {
	log.Printf("Manual check for feed %s triggered...", feedID)
	
	// Get the specific feed
	feed, err := b.feedRepo.GetFeed(feedID)
	if err != nil {
		log.Printf("Error getting feed %s: %v", feedID, err)
		return false
	}
	
	// Process this feed immediately
	b.processFeed(feed)
	return true
}

// checkAndPostNews checks all feeds and posts new articles based on schedules
func (b *Bot) checkAndPostNews() {
	// Get all registered feeds
	feeds, err := b.feedRepo.GetAllFeeds()
	if err != nil {
		log.Printf("Error getting feeds: %v", err)
		return
	}

	if len(feeds) == 0 {
		log.Println("No feeds registered")
		return
	}

	currentTime := time.Now()
	currentHourMin := currentTime.Format("15:04")

	// Process each feed independently
	for _, feed := range feeds {
		// Check if this feed should be checked now
		shouldCheck := false
		
		if len(feed.Schedule) > 0 {
			// Time-based scheduling
			for _, scheduledTime := range feed.Schedule {
				if scheduledTime == currentHourMin {
					shouldCheck = true
					log.Printf("Feed %s matches scheduled time %s", feed.ID, scheduledTime)
					break
				}
			}
		} else {
			// Fallback to interval-based (check every interval)
			// For interval fallback, we check less frequently to avoid overwhelming
			shouldCheck = (currentTime.Minute() % 15 == 0) // Check every 15 minutes
		}

		if shouldCheck {
			log.Printf("Checking feed: %s (%s)", feed.Title, feed.ID)
			b.processFeed(&feed)
		}
	}
}

// processFeed processes a single feed - checks for new articles and posts them
func (b *Bot) processFeed(feed *storage.Feed) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get channels subscribed to this feed
	channels, err := b.channelRepo.GetFeedChannels(feed.ID)
	if err != nil {
		log.Printf("Error getting channels for feed %s: %v", feed.ID, err)
		return
	}

	// Process pending queue first if channels exist
	if len(channels) > 0 {
		pendingGUIDs, err := b.historyRepo.GetPending(feed.ID)
		if err != nil {
			log.Printf("Error getting pending queue for feed %s: %v", feed.ID, err)
		} else if len(pendingGUIDs) > 0 {
			log.Printf("Processing %d pending article(s) for feed %s...", len(pendingGUIDs), feed.ID)
			for _, guid := range pendingGUIDs {
				if err := b.processPendingArticle(ctx, feed, guid, channels); err != nil {
					log.Printf("Error processing pending article %s: %v", guid, err)
				} else {
					if err := b.historyRepo.RemoveFromPending(feed.ID, guid); err != nil {
						log.Printf("Error removing from pending: %v", err)
					}
				}
			}
		}
	}

	// Create a fetcher for this feed's URL
	feedFetcher := news.NewRSSFetcher(feed.URL)
	
	// Fetch latest article from this feed
	article, err := feedFetcher.FetchLatestArticle()
	if err != nil {
		log.Printf("Error fetching article from feed %s: %v", feed.ID, err)
		return
	}

	// Check if article is new for this feed
	lastGUID, err := b.historyRepo.GetLastGUID(feed.ID)
	if err != nil {
		log.Printf("Error getting last GUID for feed %s: %v", feed.ID, err)
		return
	}

	if !news.IsNewArticle(article.GUID, lastGUID) {
		log.Printf("No new articles in feed %s", feed.ID)
		return
	}

	log.Printf("New article found in feed %s: %s", feed.ID, article.Title)

	// If no channels subscribed, add to pending queue
	if len(channels) == 0 {
		log.Printf("No channels subscribed to feed %s, adding to pending queue", feed.ID)
		if err := b.historyRepo.AddToPending(feed.ID, article.GUID); err != nil {
			log.Printf("Error adding to pending queue: %v", err)
		}
		if err := b.historyRepo.SaveGUID(feed.ID, article.GUID); err != nil {
			log.Printf("Error saving GUID: %v", err)
		}
		return
	}

	// Process and post the article
	if err := b.processAndPostArticle(ctx, feed, feedFetcher, article, channels); err != nil {
		log.Printf("Error processing article: %v", err)
	}
}

// processPendingArticle fetches and posts a pending article by GUID
func (b *Bot) processPendingArticle(ctx context.Context, feed *storage.Feed, guid string, channels []string) error {
	// Create fetcher for this feed
	feedFetcher := news.NewRSSFetcher(feed.URL)
	
	// Fetch the full feed to find this article
	articles, err := feedFetcher.FetchArticles()
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

	log.Printf("Processing pending article from feed %s: %s", feed.ID, article.Title)
	return b.processAndPostArticle(ctx, feed, feedFetcher, article, channels)
}

// processAndPostArticle scrapes, summarizes, and posts an article with multilingual support
func (b *Bot) processAndPostArticle(ctx context.Context, feed *storage.Feed, fetcher news.NewsFetcher, article *news.Article, channels []string) error {
	log.Printf("Generating summaries for %d channel(s) subscribed to feed %s...", len(channels), feed.ID)

	// Scrape article content
	content, err := fetcher.ScrapeArticleContent(article.Link)
	if err != nil {
		return fmt.Errorf("failed to scrape content: %w", err)
	}

	// Group channels by language preference
	channelsByLanguage := make(map[string][]string)
	guildLanguageCache := make(map[string]string) // Cache guild languages to avoid redundant lookups

	for _, channelID := range channels {
		// Get channel language preference
		channelLang, err := b.channelRepo.GetChannelLanguage(channelID)
		if err != nil {
			log.Printf("WARNING: Failed to get language for channel %s: %v, using default", channelID, err)
			channelLang = "" // Will fall back to guild default
		}

		// If no channel-specific language, use guild default
		if channelLang == "" {
			// Get guild ID from channel
			channel, err := b.session.Channel(channelID)
			if err != nil {
				log.Printf("WARNING: Failed to get channel info for %s: %v, using en", channelID, err)
				channelLang = "en"
			} else {
				guildID := channel.GuildID
				
				// Check cache first
				if cachedLang, ok := guildLanguageCache[guildID]; ok {
					channelLang = cachedLang
				} else {
					// Fetch and cache guild language
					guildLang, err := b.channelRepo.GetGuildLanguage(guildID)
					if err != nil {
						log.Printf("WARNING: Failed to get guild language for %s: %v, using en", guildID, err)
						guildLang = "en"
					}
					if guildLang == "" {
						guildLang = "en"
					}
					guildLanguageCache[guildID] = guildLang
					channelLang = guildLang
				}
			}
		}

		log.Printf("Channel %s will receive summary in: %s", channelID, channelLang)
		channelsByLanguage[channelLang] = append(channelsByLanguage[channelLang], channelID)
	}

	log.Printf("Grouped channels into %d language(s): %v", len(channelsByLanguage), getLanguageList(channelsByLanguage))

	// Generate one summary per language
	summariesByLanguage := make(map[string]string)
	totalSuccessCount := 0

	for lang, langChannels := range channelsByLanguage {
		log.Printf("Generating summary in %s for %d channel(s)...", lang, len(langChannels))
		
		summary, err := b.aiSummarizer.SummarizeInLanguage(ctx, content, lang)
		if err != nil {
			log.Printf("ERROR: Failed to generate summary in %s: %v", lang, err)
			// Try fallback to English if primary language fails
			if lang != "en" {
				log.Printf("Attempting fallback to English for %s channels", lang)
				summary, err = b.aiSummarizer.SummarizeInLanguage(ctx, content, "en")
				if err != nil {
					log.Printf("ERROR: English fallback also failed: %v", err)
					continue
				}
				log.Printf("Successfully generated English fallback summary")
			} else {
				continue
			}
		}

		summariesByLanguage[lang] = summary
		log.Printf("Summary generated in %s: %s", lang, article.Title)

		// Create embed message with feed info
		embed := b.createNewsEmbed(feed, article, summary)

		// Broadcast to all channels using this language
		successCount := 0
		for _, channelID := range langChannels {
			if err := b.sendEmbed(channelID, embed); err != nil {
				log.Printf("Error sending to channel %s: %v", channelID, err)
			} else {
				successCount++
			}
		}

		log.Printf("Article in %s posted to %d/%d channels", lang, successCount, len(langChannels))
		totalSuccessCount += successCount
	}

	log.Printf("Article posted to %d/%d total channels across %d language(s) for feed %s", 
		totalSuccessCount, len(channels), len(summariesByLanguage), feed.ID)

	// Save GUID to history for this feed
	if err := b.historyRepo.SaveGUID(feed.ID, article.GUID); err != nil {
		return fmt.Errorf("failed to save GUID: %w", err)
	}

	return nil
}

// getLanguageList returns a list of language codes from the channelsByLanguage map
func getLanguageList(channelsByLanguage map[string][]string) []string {
	languages := make([]string, 0, len(channelsByLanguage))
	for lang := range channelsByLanguage {
		languages = append(languages, lang)
	}
	return languages
}

// createNewsEmbed creates a Discord embed for the news article with feed info
func (b *Bot) createNewsEmbed(feed *storage.Feed, article *news.Article, summary string) *discordgo.MessageEmbed {
	// Use feed description for footer, or just feed title if no description
	footerText := feed.Title
	if feed.Description != "" {
		footerText = fmt.Sprintf("%s • %s", feed.Title, feed.Description)
	}
	
	return &discordgo.MessageEmbed{
		Title:       article.Title,
		Description: summary,
		URL:         article.Link,
		Color:       0x478CBF, // Blue color
		Footer: &discordgo.MessageEmbedFooter{
			Text: footerText,
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
