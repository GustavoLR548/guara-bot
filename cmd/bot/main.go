package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ai"
	"github.com/GustavoLR548/godot-news-bot/internal/bot"
	"github.com/GustavoLR548/godot-news-bot/internal/github"
	"github.com/GustavoLR548/godot-news-bot/internal/news"
	"github.com/GustavoLR548/godot-news-bot/internal/ratelimit"
	"github.com/GustavoLR548/godot-news-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

const (
	defaultMaxChannels          = 5
	defaultCheckIntervalMinutes = 15
	rssURL                      = "https://godotengine.org/rss.xml"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Get configuration from environment
	discordToken := os.Getenv("DISCORD_TOKEN")
	if discordToken == "" {
		log.Fatal("DISCORD_TOKEN is required")
	}

	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	// GitHub is optional - if no token provided, GitHub monitoring will be disabled

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379" // default
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	maxChannels := getEnvAsInt("MAX_CHANNELS_LIMIT", defaultMaxChannels)
	checkIntervalMinutes := getEnvAsInt("CHECK_INTERVAL_MINUTES", defaultCheckIntervalMinutes)
	checkInterval := time.Duration(checkIntervalMinutes) * time.Minute

	// Configure rate limiting
	rateLimitConfig := ratelimit.Config{
		MaxRequestsPerMinute:    getEnvAsInt("GEMINI_MAX_REQUESTS_PER_MINUTE", 10),
		MaxTokensPerMinute:      getEnvAsInt("GEMINI_MAX_TOKENS_PER_MINUTE", 200000),
		MaxTokensPerRequest:     getEnvAsInt("GEMINI_MAX_TOKENS_PER_REQUEST", 4000),
		CircuitBreakerThreshold: getEnvAsInt("GEMINI_CIRCUIT_BREAKER_THRESHOLD", 5),
		CircuitBreakerTimeout:   time.Duration(getEnvAsInt("GEMINI_CIRCUIT_BREAKER_TIMEOUT_MINUTES", 5)) * time.Minute,
		RetryAttempts:           getEnvAsInt("GEMINI_RETRY_ATTEMPTS", 3),
		RetryBackoffBase:        time.Duration(getEnvAsInt("GEMINI_RETRY_BACKOFF_SECONDS", 1)) * time.Second,
	}

	log.Printf("Starting Guara Bot (Max Channels: %d, Check Interval: %v)", maxChannels, checkInterval)
	log.Printf("Rate Limiting: %d RPM, %d TPM, Circuit Breaker: %d failures", 
		rateLimitConfig.MaxRequestsPerMinute, 
		rateLimitConfig.MaxTokensPerMinute,
		rateLimitConfig.CircuitBreakerThreshold)

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPassword,
		DB:       0,
	})

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	// Initialize storage repositories
	channelRepo, err := storage.NewRedisChannelRepository(redisClient, maxChannels)
	if err != nil {
		log.Fatalf("Failed to create channel repository: %v", err)
	}

	historyRepo := storage.NewRedisHistoryRepository(redisClient)
	feedRepo := storage.NewRedisFeedRepository(redisClient)
	githubRepo := storage.NewRedisGitHubRepository(redisClient)

	// Register default feed for backward compatibility
	defaultFeed := storage.Feed{
		ID:          "godot-official",
		URL:         rssURL,
		Title:       "Godot Engine Official",
		Description: "Official Godot Engine news and announcements",
		AddedAt:     time.Now(),
		Schedule:    []string{}, // Empty schedule for now (will use check interval)
	}
	
	log.Printf("Checking if default feed exists: %s", defaultFeed.ID)
	// Only register if it doesn't exist
	if has, err := feedRepo.HasFeed(defaultFeed.ID); err != nil {
		log.Printf("ERROR: Failed to check if feed exists: %v", err)
	} else if !has {
		log.Printf("Default feed does not exist, registering: %s", defaultFeed.ID)
		if err := feedRepo.RegisterFeed(defaultFeed); err != nil {
			log.Printf("ERROR: Failed to register default feed: %v", err)
		} else {
			log.Printf("SUCCESS: Registered default feed: %s (URL: %s)", defaultFeed.ID, defaultFeed.URL)
		}
	} else {
		log.Printf("Default feed already exists: %s", defaultFeed.ID)
	}

	// Initialize news fetcher
	newsFetcher := news.NewRSSFetcher(rssURL)

	// Initialize AI summarizer with rate limiting
	aiSummarizer := ai.NewGeminiSummarizerWithRateLimit(geminiAPIKey, rateLimitConfig)

	// Initialize GitHub client if token is provided
	var githubClient *github.Client
	var githubMonitor *bot.GitHubMonitor
	if githubToken != "" {
		log.Println("GitHub token provided, enabling GitHub PR monitoring")
		githubClient = github.NewClient(githubToken)
		
		// Create PR summarizer
		prSummarizer := ai.NewGeminiPRSummarizer(aiSummarizer)
		
		// Note: githubMonitor will be initialized after Discord session is created
		_ = prSummarizer // We'll use this later
	} else {
		log.Println("No GitHub token provided, GitHub PR monitoring disabled")
	}

	// Create Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	// Create command handler with GitHub repo
	commandHandler := bot.NewCommandHandler(channelRepo, feedRepo, githubRepo, maxChannels)

	// Initialize GitHub monitor if enabled
	if githubClient != nil {
		prSummarizer := ai.NewGeminiPRSummarizer(aiSummarizer)
		githubMonitor = bot.NewGitHubMonitor(dg, githubClient, githubRepo, prSummarizer)
	}

	// Register commands and handlers
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
		log.Printf("Bot ID: %v", s.State.User.ID)
		
		// Register slash commands
		if err := commandHandler.RegisterCommands(s); err != nil {
			log.Printf("Error registering commands: %v", err)
		}
	})

	// Handle commands
	commandHandler.HandleCommands(dg)

	// Open Discord connection
	if err := dg.Open(); err != nil {
		log.Fatalf("Failed to open Discord connection: %v", err)
	}
	defer dg.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")

	// Create and start the bot
	newsBot := bot.NewBot(
		dg,
		newsFetcher,
		aiSummarizer,
		channelRepo,
		historyRepo,
		feedRepo,
		checkInterval,
	)

	// Connect bot to command handler
	commandHandler.SetBot(newsBot)

	// Connect GitHub monitor to command handler if enabled
	if githubMonitor != nil {
		commandHandler.SetGitHubMonitor(githubMonitor)
	}

	// Start news loop in goroutine
	go newsBot.Start()

	// Start GitHub monitoring if enabled
	if githubMonitor != nil {
		log.Println("Starting GitHub PR monitoring...")
		go githubMonitor.Start(context.Background())
	}

	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("Shutting down...")
	newsBot.Stop()
	
	// Close Redis connection
	if err := redisClient.Close(); err != nil {
		log.Printf("Error closing Redis connection: %v", err)
	}
	
	// Cleanup commands (optional, but good practice)
	cleanupCommands(dg)
}

// getEnvAsInt retrieves an environment variable as an integer with a default value
func getEnvAsInt(key string, defaultVal int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}

	val, err := strconv.Atoi(valStr)
	if err != nil {
		log.Printf("Warning: Invalid value for %s, using default: %d", key, defaultVal)
		return defaultVal
	}

	return val
}

// cleanupCommands removes all registered commands on shutdown
func cleanupCommands(s *discordgo.Session) {
	log.Println("Cleaning up commands...")

	commands, err := s.ApplicationCommands(s.State.User.ID, "")
	if err != nil {
		log.Printf("Error fetching commands: %v", err)
		return
	}

	for _, cmd := range commands {
		err := s.ApplicationCommandDelete(s.State.User.ID, "", cmd.ID)
		if err != nil {
			log.Printf("Error deleting command %s: %v", cmd.Name, err)
		} else {
			log.Printf("Deleted command: %s", cmd.Name)
		}
	}
}
