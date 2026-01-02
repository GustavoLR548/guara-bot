package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/GustavoLR548/godot-news-bot/internal/ai"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	fmt.Println("🔍 Listing available Gemini models...")
	fmt.Println()

	// Use default rate limiting for this utility
	summarizer := ai.NewGeminiSummarizer(geminiAPIKey)
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, err := summarizer.ListAvailableModels(ctx)
	if err != nil {
		log.Fatalf("Error listing models: %v", err)
	}

	if len(models) == 0 {
		fmt.Println("❌ No models found")
		return
	}

	fmt.Printf("✅ Found %d available model(s):\n", len(models))
	for i, model := range models {
		fmt.Printf("%d. %s\n", i+1, model)
	}

	fmt.Println("\n💡 Free tier models typically include:")
	fmt.Println("   - gemini-1.5-flash")
	fmt.Println("   - gemini-1.5-flash-8b")
	fmt.Println("   - gemini-pro")
	fmt.Println("\n📝 Update internal/ai/gemini.go with the correct model name")
}
