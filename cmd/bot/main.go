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
	"github.com/GustavoLR548/godot-news-bot/internal/news"
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

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379" // default
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	maxChannels := getEnvAsInt("MAX_CHANNELS_LIMIT", defaultMaxChannels)
	checkIntervalMinutes := getEnvAsInt("CHECK_INTERVAL_MINUTES", defaultCheckIntervalMinutes)
	checkInterval := time.Duration(checkIntervalMinutes) * time.Minute

	log.Printf("Starting Godot News Bot (Max Channels: %d, Check Interval: %v)", maxChannels, checkInterval)

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

	// Initialize news fetcher
	newsFetcher := news.NewRSSFetcher(rssURL)

	// Initialize AI summarizer
	aiSummarizer := ai.NewGeminiSummarizer(geminiAPIKey)

	// Create Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	// Create command handler
	commandHandler := bot.NewCommandHandler(channelRepo, maxChannels)

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
		checkInterval,
	)

	// Connect bot to command handler
	commandHandler.SetBot(newsBot)

	// Start news loop in goroutine
	go newsBot.Start()

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
