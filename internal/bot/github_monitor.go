package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ai"
	"github.com/GustavoLR548/godot-news-bot/internal/github"
	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// GitHubMonitor monitors GitHub repositories for new PRs
type GitHubMonitor struct {
	session        *discordgo.Session
	githubClient   *github.Client
	githubRepo     storage.GitHubRepository
	summarizer     ai.PRSummarizer
	checkInterval  time.Duration
	batchThreshold int
}

// NewGitHubMonitor creates a new GitHub monitor
func NewGitHubMonitor(
	session *discordgo.Session,
	githubClient *github.Client,
	githubRepo storage.GitHubRepository,
	summarizer ai.PRSummarizer,
) *GitHubMonitor {
	// Get check interval from environment (default 30 minutes)
	checkIntervalStr := os.Getenv("GITHUB_CHECK_INTERVAL_MINUTES")
	checkInterval := 30 * time.Minute
	if checkIntervalStr != "" {
		if minutes, err := strconv.Atoi(checkIntervalStr); err == nil && minutes > 0 {
			checkInterval = time.Duration(minutes) * time.Minute
		}
	}

	// Get batch threshold from environment (default 5)
	batchThresholdStr := os.Getenv("GITHUB_BATCH_THRESHOLD")
	batchThreshold := 5
	if batchThresholdStr != "" {
		if threshold, err := strconv.Atoi(batchThresholdStr); err == nil && threshold > 0 {
			batchThreshold = threshold
		}
	}

	return &GitHubMonitor{
		session:        session,
		githubClient:   githubClient,
		githubRepo:     githubRepo,
		summarizer:     summarizer,
		checkInterval:  checkInterval,
		batchThreshold: batchThreshold,
	}
}

// Start begins monitoring repositories
func (m *GitHubMonitor) Start(ctx context.Context) {
	log.Printf("[GITHUB-MONITOR] Starting with check interval: %v, batch threshold: %d", m.checkInterval, m.batchThreshold)

	// Check every minute to see if any schedules match current time
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Run initial check immediately (for repos without schedules)
	m.checkAllRepositories(ctx, false)

	for {
		select {
		case <-ctx.Done():
			log.Println("[GITHUB-MONITOR] Stopping...")
			return
		case <-ticker.C:
			// Get current time in HH:MM format
			now := time.Now().Format("15:04")
			m.checkScheduledRepositories(ctx, now)
		}
	}
}

// CheckAllRepositoriesNow forces an immediate check of all repositories (for manual triggers)
func (m *GitHubMonitor) CheckAllRepositoriesNow() {
	log.Println("[GITHUB-MONITOR] Manual check triggered for all repositories")
	go m.checkAllRepositories(context.Background(), false)
}

// CheckRepositoryNow forces an immediate check of a specific repository (for manual triggers)
func (m *GitHubMonitor) CheckRepositoryNow(repoID string) {
	log.Printf("[GITHUB-MONITOR] Manual check triggered for repository: %s", repoID)
	
	repo, err := m.githubRepo.GetRepository(repoID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get repository %s: %v", repoID, err)
		return
	}
	
	go m.checkRepository(context.Background(), *repo)
}

// ProcessPendingPRsNow forces immediate processing of pending PRs for a repository
func (m *GitHubMonitor) ProcessPendingPRsNow(repoID string) {
	log.Printf("[GITHUB-MONITOR] Manual processing of pending PRs triggered for repository: %s", repoID)
	
	repo, err := m.githubRepo.GetRepository(repoID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get repository %s: %v", repoID, err)
		return
	}
	
	// Check if there are pending PRs
	pendingCount, err := m.githubRepo.GetPendingCount(repoID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get pending count for %s: %v", repoID, err)
		return
	}
	
	if pendingCount == 0 {
		log.Printf("[GITHUB-MONITOR] No pending PRs for %s", repoID)
		return
	}
	
	log.Printf("[GITHUB-MONITOR] Processing %d pending PRs for %s", pendingCount, repoID)
	m.processBatch(context.Background(), *repo)
}

// checkScheduledRepositories checks repos whose schedules match the current time
func (m *GitHubMonitor) checkScheduledRepositories(ctx context.Context, currentTime string) {
	repos, err := m.githubRepo.GetAllRepositories()
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get repositories: %v", err)
		return
	}

	for _, repo := range repos {
		select {
		case <-ctx.Done():
			return
		default:
			// Get schedule for this repo
			schedule, _ := m.githubRepo.GetSchedule(repo.ID)
			
			// If no schedule, check based on interval
			if len(schedule) == 0 {
				// Check if enough time has passed since last check
				lastChecked, _ := m.githubRepo.GetLastChecked(repo.ID)
				if time.Since(lastChecked) >= m.checkInterval {
					m.checkRepository(ctx, repo)
				}
				continue
			}
			
			// Check if current time matches any scheduled time
			for _, scheduledTime := range schedule {
				if scheduledTime == currentTime {
					log.Printf("[GITHUB-MONITOR] Scheduled check for %s/%s at %s", repo.Owner, repo.Name, currentTime)
					m.checkRepository(ctx, repo)
					break
				}
			}
		}
	}
}

// checkAllRepositories checks all registered repositories (legacy method for interval-based)
func (m *GitHubMonitor) checkAllRepositories(ctx context.Context, respectSchedules bool) {
	repos, err := m.githubRepo.GetAllRepositories()
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get repositories: %v", err)
		return
	}

	if len(repos) == 0 {
		log.Println("[GITHUB-MONITOR] No repositories registered")
		return
	}

	log.Printf("[GITHUB-MONITOR] Checking %d repositories...", len(repos))

	for _, repo := range repos {
		select {
		case <-ctx.Done():
			return
		default:
			// Skip if repo has schedule and we're respecting schedules
			if respectSchedules && len(repo.Schedule) > 0 {
				continue
			}
			m.checkRepository(ctx, repo)
		}
	}
}

// checkRepository checks a single repository for new PRs
func (m *GitHubMonitor) checkRepository(ctx context.Context, repo github.Repository) {
	log.Printf("[GITHUB-MONITOR] Checking repository: %s/%s", repo.Owner, repo.Name)

	// Get last checked time
	lastChecked, err := m.githubRepo.GetLastChecked(repo.ID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get last checked time for %s: %v", repo.ID, err)
		lastChecked = time.Now().Add(-24 * time.Hour) // Default to 24 hours ago
	}

	// Limit lookback to 3 days max to avoid processing too many PRs at once
	maxLookback := time.Now().Add(-3 * 24 * time.Hour)
	if lastChecked.IsZero() || lastChecked.Before(maxLookback) {
		lastChecked = maxLookback
		log.Printf("[GITHUB-MONITOR] Limiting lookback to 3 days for %s", repo.ID)
	}

	// Fetch merged PRs since last check
	prs, err := m.githubClient.FetchMergedPRs(ctx, repo.Owner, repo.Name, repo.TargetBranch, lastChecked)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to fetch PRs for %s/%s: %v", repo.Owner, repo.Name, err)
		return
	}

	if len(prs) == 0 {
		log.Printf("[GITHUB-MONITOR] No new PRs for %s/%s", repo.Owner, repo.Name)
		m.githubRepo.UpdateLastChecked(repo.ID, time.Now())
		
		// Even if no new PRs, check if there are pending PRs to process
		pendingCount, err := m.githubRepo.GetPendingCount(repo.ID)
		if err != nil {
			log.Printf("[GITHUB-MONITOR] ERROR: Failed to get pending count: %v", err)
			return
		}
		
		if pendingCount >= m.batchThreshold {
			log.Printf("[GITHUB-MONITOR] Found %d pending PRs in queue, processing batch", pendingCount)
			m.processBatch(ctx, repo)
		} else if pendingCount > 0 {
			log.Printf("[GITHUB-MONITOR] %d PRs in queue (threshold: %d), waiting for more", pendingCount, m.batchThreshold)
		}
		return
	}

	log.Printf("[GITHUB-MONITOR] Found %d merged PRs for %s/%s", len(prs), repo.Owner, repo.Name)

	// Filter and process PRs
	highValueCount := 0
	rejectedCount := 0
	alreadyProcessedCount := 0
	
	for _, pr := range prs {
		// Check if already processed
		processed, err := m.githubRepo.IsProcessed(repo.ID, pr.ID)
		if err != nil {
			log.Printf("[GITHUB-MONITOR] ERROR: Failed to check if PR %d is processed: %v", pr.ID, err)
			continue
		}
		if processed {
			alreadyProcessedCount++
			continue
		}

		// Fetch PR files for filtering
		files, err := m.githubClient.FetchPRFiles(ctx, repo.Owner, repo.Name, pr.Number)
		if err != nil {
			log.Printf("[GITHUB-MONITOR] ERROR: Failed to fetch files for PR #%d: %v", pr.Number, err)
			continue
		}
		pr.Files = files

		// Check if high-value PR
		filterConfig := github.DefaultFilterConfig()
		if !github.IsHighValuePR(pr, filterConfig) {
			rejectedCount++
			m.githubRepo.MarkProcessed(repo.ID, pr.ID)
			continue
		}

		// Categorize PR (we don't need to store it, AI will categorize later)
		_ = github.CategorizePR(pr)

		// Add to pending queue
		if err := m.githubRepo.AddToPendingQueue(repo.ID, pr); err != nil {
			log.Printf("[GITHUB-MONITOR] ERROR: Failed to add PR to pending queue: %v", err)
			continue
		}

		// Mark as processed
		m.githubRepo.MarkProcessed(repo.ID, pr.ID)
		highValueCount++
	}

	log.Printf("[GITHUB-MONITOR] ========================================")
	log.Printf("[GITHUB-MONITOR] PR Filtering Summary for %s/%s:", repo.Owner, repo.Name)
	log.Printf("[GITHUB-MONITOR]   Total fetched: %d", len(prs))
	log.Printf("[GITHUB-MONITOR]   Already processed: %d", alreadyProcessedCount)
	log.Printf("[GITHUB-MONITOR]   Filtered out (rejected): %d", rejectedCount)
	log.Printf("[GITHUB-MONITOR]   Accepted (high-value): %d", highValueCount)
	log.Printf("[GITHUB-MONITOR] ========================================")

	// Update last checked time
	m.githubRepo.UpdateLastChecked(repo.ID, time.Now())

	// Check if we should process the batch
	pendingCount, err := m.githubRepo.GetPendingCount(repo.ID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get pending count: %v", err)
		return
	}

	if pendingCount >= m.batchThreshold {
		log.Printf("[GITHUB-MONITOR] Batch threshold reached (%d >= %d), processing batch for %s/%s",
			pendingCount, m.batchThreshold, repo.Owner, repo.Name)
		m.processBatch(ctx, repo)
	}
}

// processBatch generates a summary and posts to all subscribed channels
// Processes only ONE batch at a time (respects batch_threshold)
func (m *GitHubMonitor) processBatch(ctx context.Context, repo github.Repository) {
	// Get pending count first
	pendingCount, err := m.githubRepo.GetPendingCount(repo.ID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get pending count for %s: %v", repo.ID, err)
		return
	}
	
	if pendingCount == 0 {
		log.Printf("[GITHUB-MONITOR] No pending PRs to process for %s", repo.ID)
		return
	}
	
	// Get all pending PRs
	allPendingPRs, err := m.githubRepo.GetPendingQueue(repo.ID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get pending queue for %s: %v", repo.ID, err)
		return
	}

	if len(allPendingPRs) == 0 {
		log.Printf("[GITHUB-MONITOR] No PRs in pending queue for %s", repo.ID)
		return
	}

	// Process only up to batch_threshold PRs at a time to avoid token limits
	var prs []github.PullRequest
	if len(allPendingPRs) > m.batchThreshold {
		prs = allPendingPRs[:m.batchThreshold]
		log.Printf("[GITHUB-MONITOR] Batch threshold reached (%d >= %d), processing batch for %s/%s",
			pendingCount, m.batchThreshold, repo.Owner, repo.Name)
		log.Printf("[GITHUB-MONITOR] Processing batch of %d PRs (out of %d pending) from %s/%s",
			m.batchThreshold, len(allPendingPRs), repo.Owner, repo.Name)
	} else {
		prs = allPendingPRs
		log.Printf("[GITHUB-MONITOR] Processing final batch of %d PRs from %s/%s",
			len(prs), repo.Owner, repo.Name)
	}

	// Get subscribed channels
	channels, err := m.githubRepo.GetRepoChannels(repo.ID)
	if err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to get channels for %s: %v", repo.ID, err)
		return
	}

	if len(channels) == 0 {
		log.Printf("[GITHUB-MONITOR] No channels subscribed to %s yet, keeping %d PRs in queue for later", repo.ID, len(prs))
		return
	}

	log.Printf("[GITHUB-MONITOR] Posting summary to %d channels", len(channels))

	// Group channels by language for efficient AI generation
	channelsByLang := make(map[string][]string)
	guildLanguageCache := make(map[string]string)

	for _, channelID := range channels {
		// Detect language for this channel
		language := m.detectChannelLanguage(channelID, guildLanguageCache)
		channelsByLang[language] = append(channelsByLang[language], channelID)
		log.Printf("[GITHUB-MONITOR] Channel %s will receive summary in %s", channelID, language)
	}

	log.Printf("[GITHUB-MONITOR] Grouped %d channels into %d languages", len(channels), len(channelsByLang))

	// Determine optimal batch size (use first language for estimation)
	firstLang := ""
	for lang := range channelsByLang {
		firstLang = lang
		break
	}
	
	maxTokens := 30000 // Conservative limit
	optimalCount := ai.FitPRsWithinTokenLimit(prs, firstLang, maxTokens)
	
	if optimalCount < len(prs) {
		log.Printf("[GITHUB-MONITOR] Token limit: processing %d/%d PRs, %d will be deferred", optimalCount, len(prs), len(prs)-optimalCount)
		
		// Return overflow PRs back to queue
		overflowPRs := prs[optimalCount:]
		for i := len(overflowPRs) - 1; i >= 0; i-- {
			if err := m.githubRepo.AddToPendingQueue(repo.ID, overflowPRs[i]); err != nil {
				log.Printf("[GITHUB-MONITOR] ERROR: Failed to return PR #%d to queue: %v", overflowPRs[i].Number, err)
			}
		}
		log.Printf("[GITHUB-MONITOR] Returned %d PRs to queue for next batch", len(overflowPRs))
		
		// Process only the PRs that fit
		prs = prs[:optimalCount]
	}

	// Generate summary once per language and post to all channels in that language
	repoName := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
	totalSuccess := 0

	for language, langChannels := range channelsByLang {
		log.Printf("[GITHUB-MONITOR] Generating %s summary for %d PRs, %d channels", language, len(prs), len(langChannels))

		// Generate summary using AI
		summaryText, err := m.summarizer.SummarizePRBatch(ctx, repoName, prs, language)
		if err != nil {
			log.Printf("[GITHUB-MONITOR] ERROR: Failed to generate %s summary: %v", language, err)
			continue
		}

		// Post to all channels in this language group
		successCount := 0
		for _, channelID := range langChannels {
			if err := m.postSummaryToChannel(channelID, summaryText, repo, len(prs), language); err != nil {
				log.Printf("[GITHUB-MONITOR] ERROR: Failed to post to channel %s: %v", channelID, err)
				continue
			}
			successCount++
		}

		log.Printf("[GITHUB-MONITOR] Posted %s summary to %d/%d channels", language, successCount, len(langChannels))
		totalSuccess += successCount
	}

	log.Printf("[GITHUB-MONITOR] Posted summary to %d/%d channels total", totalSuccess, len(channels))

	// Remove only the PRs we processed from the queue
	if err := m.githubRepo.RemoveFromPendingQueue(repo.ID, len(prs)); err != nil {
		log.Printf("[GITHUB-MONITOR] ERROR: Failed to remove processed PRs from queue: %v", err)
	} else {
		log.Printf("[GITHUB-MONITOR] Removed %d processed PRs from queue", len(prs))
	}
}

// detectChannelLanguage detects the language for a channel using the same hierarchy as RSS feeds
func (m *GitHubMonitor) detectChannelLanguage(channelID string, guildLanguageCache map[string]string) string {
	// Try to get channel-specific language
	channelLang, err := m.githubRepo.GetChannelLanguage(channelID)
	if err == nil && channelLang != "" {
		return channelLang
	}

	// Try to get guild language
	channel, err := m.session.Channel(channelID)
	if err == nil && channel.GuildID != "" {
		// Check cache first
		if cachedLang, ok := guildLanguageCache[channel.GuildID]; ok {
			return cachedLang
		}

		// Fetch from storage
		guildLang, err := m.githubRepo.GetGuildLanguage(channel.GuildID)
		if err == nil && guildLang != "" {
			guildLanguageCache[channel.GuildID] = guildLang
			return guildLang
		}
	}

	// Default to English
	return "en"
}

// postSummaryToChannel posts a PR summary to a Discord channel
func (m *GitHubMonitor) postSummaryToChannel(channelID string, summaryText string, repo github.Repository, prCount int, language string) error {
	// Localize title and footer based on language
	var title, footerText string
	langInfo := ai.GetLanguageInfo(language)
	
	switch language {
	case "pt-BR":
		title = fmt.Sprintf("🔄 Resumo de Pull Requests: %s/%s", repo.Owner, repo.Name)
		footerText = fmt.Sprintf("Resumo de %d PRs mesclados do branch %s", prCount, repo.TargetBranch)
	case "es":
		title = fmt.Sprintf("🔄 Resumen de Pull Requests: %s/%s", repo.Owner, repo.Name)
		footerText = fmt.Sprintf("Resumen de %d PRs fusionados de la rama %s", prCount, repo.TargetBranch)
	case "fr":
		title = fmt.Sprintf("🔄 Résumé des Pull Requests: %s/%s", repo.Owner, repo.Name)
		footerText = fmt.Sprintf("Résumé de %d PRs fusionnées de la branche %s", prCount, repo.TargetBranch)
	case "de":
		title = fmt.Sprintf("🔄 Pull Request Zusammenfassung: %s/%s", repo.Owner, repo.Name)
		footerText = fmt.Sprintf("Zusammenfassung von %d gemergten PRs vom Branch %s", prCount, repo.TargetBranch)
	case "ja":
		title = fmt.Sprintf("🔄 プルリクエストの要約: %s/%s", repo.Owner, repo.Name)
		footerText = fmt.Sprintf("%s ブランチからマージされた %d 件のPRの要約", repo.TargetBranch, prCount)
	default: // English
		title = fmt.Sprintf("🔄 Pull Request Summary: %s/%s", repo.Owner, repo.Name)
		footerText = fmt.Sprintf("Summarized %d merged PRs from %s branch", prCount, repo.TargetBranch)
	}
	
	log.Printf("[GITHUB-MONITOR] Posting summary in %s (%s) to channel %s", langInfo.Name, language, channelID)
	
	// Discord embed description has a 6000 character limit
	const maxEmbedDescriptionLength = 6000
	if len(summaryText) > maxEmbedDescriptionLength {
		log.Printf("[GITHUB-MONITOR] WARNING: Summary too long (%d chars), truncating to %d chars", len(summaryText), maxEmbedDescriptionLength)
		// Truncate and add ellipsis
		summaryText = summaryText[:maxEmbedDescriptionLength-100] + "\n\n...\n\n⚠️ Summary truncated due to length. Check GitHub for full details."
	}
	
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: summaryText,
		Color:       0x6E5494, // GitHub purple
		Footer: &discordgo.MessageEmbedFooter{
			Text: footerText,
		},
		Timestamp: time.Now().Format(time.RFC3339),
		URL:       fmt.Sprintf("https://github.com/%s/%s/pulls?q=is:pr+is:merged", repo.Owner, repo.Name),
	}

	_, err := m.session.ChannelMessageSendEmbed(channelID, embed)
	return err
}
