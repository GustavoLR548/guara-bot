# Guara Bot

A versatile Discord bot that monitors multiple RSS feeds and posts AI-generated summaries in Brazilian Portuguese. Perfect for keeping your community updated with news from blogs, game engines, tech sites, and any RSS-enabled source.

## Features

- 📰 **Multiple RSS feed support** - Register and monitor unlimited RSS feeds
- ⏰ **Time-based scheduling** - Set specific check times per feed (HH:MM format)
- 🤖 Automatic RSS monitoring with flexible per-feed scheduling
- 🧠 AI-powered summaries using Google Gemini 2.5 Flash
- 🇧🇷 Summaries in Brazilian Portuguese
- 📢 Multi-channel support with configurable limits
- 🔗 Many-to-many architecture: channels can subscribe to multiple feeds
- 🎯 Per-feed customization: different schedules, different channels
- 🔒 Admin-only feed and channel management via slash commands
- 💾 Redis-backed persistence with per-feed pending queues
- 🛡️ **Cost management with rate limiting and circuit breaker**
- 🔄 **Automatic retry logic with exponential backoff**
- 📊 **Token counting to prevent API quota overruns**
- ✅ Fully tested with TDD architecture (55 tests across all packages)

## Quick Start

### Prerequisites
- Go 1.23+
- Redis Server
- Discord Bot Token
- Google Gemini API Key

### Installation

1. **Clone and setup:**
```bash
git clone <repository-url>
cd guara-bot
cp .env.example .env
```

2. **Configure `.env`:**
```env
DISCORD_TOKEN=your_discord_bot_token
GEMINI_API_KEY=your_gemini_api_key
MAX_CHANNELS_LIMIT=5
CHECK_INTERVAL_MINUTES=15  # Fallback for feeds without schedules
REDIS_URL=localhost:6379
REDIS_PASSWORD=

# Rate Limiting (Gemini Free Tier Protection)
GEMINI_MAX_REQUESTS_PER_MINUTE=10
GEMINI_MAX_TOKENS_PER_MINUTE=200000
GEMINI_MAX_TOKENS_PER_REQUEST=4000
GEMINI_CIRCUIT_BREAKER_THRESHOLD=5
GEMINI_CIRCUIT_BREAKER_TIMEOUT_MINUTES=5
GEMINI_RETRY_ATTEMPTS=3
GEMINI_RETRY_BACKOFF_SECONDS=1
```

3. **Run with Docker:**
```bash
docker-compose up -d
```

Or run locally:
```bash
go run ./cmd/bot
```

For more information, please check out [here](QUICKSTART.md)

## Usage

### Managing Feeds

```bash
# Register RSS feeds from various sources
/register-feed godot https://godotengine.org/rss.xml "Godot Engine" "Game engine news"
/register-feed gdquest https://www.gdquest.com/rss.xml "GDQuest" "Godot tutorials"
/register-feed techcrunch https://techcrunch.com/feed/ "TechCrunch" "Tech news"
/register-feed dev-to https://dev.to/feed "DEV Community" "Developer articles"

# List all registered feeds
/list-feeds

# Set check times for each feed (9 AM, 1 PM, 6 PM)
/schedule-feed godot 09:00,18:00
/schedule-feed techcrunch 08:00,12:00,17:00

# Remove a feed
/unregister-feed gdquest
```

### Managing Channels

```bash
# Subscribe channels to specific feeds
/setup-news #game-news godot
/setup-news #tutorials gdquest
/setup-news #tech-news techcrunch

# A single channel can subscribe to multiple feeds
/setup-news #general godot
/setup-news #general techcrunch
/setup-news #general dev-to

# Unsubscribe from a feed
/remove-news #tutorials gdquest

# Force immediate check of a specific feed
/update-news godot
/update-news techcrunch

# Force immediate check of all registered feeds
/update-all-news
```

### Default Feed

The bot automatically creates a default feed called `godot-official` pointing to Godot Engine news for backward compatibility. You can remove it and add your own feeds as needed.

## Cost Management & Rate Limiting

The bot includes comprehensive cost management to protect against exceeding Gemini API free tier limits:

### Features
- **Token Counting**: Estimates input/output tokens before making requests
- **Rate Limiting**: Enforces 10 RPM and 200k TPM limits (configurable)
- **Circuit Breaker**: Temporarily stops requests after 5 consecutive failures
- **Retry Logic**: Automatic retries with exponential backoff (1s, 2s, 4s, etc.)
- **Wait for Capacity**: Automatically waits when limits are reached

### Gemini Free Tier Limits
- 15 requests per minute (RPM)
- 250,000 tokens per minute (TPM)
- Bot defaults to conservative 10 RPM / 200k TPM

### Configuration
All rate limits are configurable via environment variables (see `.env.example`). The defaults are set conservatively to ensure you stay well within the free tier limits.

### Monitoring
Check logs for rate limiting information:
```
Rate Limiting: 10 RPM, 200000 TPM, Circuit Breaker: 5 failures
Token estimate: input=1234, estimated_output=1500, total=2734
Request successful, recorded 2850 tokens
```

## Contributing

Contributions are welcome! Please:
1. Write tests for new features
2. Follow Go conventions
3. Use table-driven tests
4. Keep interfaces for testability
5. Run `go fmt` before committing

## Troubleshooting

### Bot doesn't respond to commands
- Ensure bot has proper Discord permissions
- Check bot token is correct
- Verify bot is invited with `applications.commands` scope

### News not posting
- Use `/list-feeds` to verify feeds are registered
- Use `/list-channels` to ensure channels are subscribed
- Check feed schedules with `/list-feeds` or set them with `/schedule-feed`
- Verify feed URL is accessible: `curl <feed-url>`
- Verify Gemini API key is valid
- Use `/update-news` to trigger immediate check
- Check logs for error messages
- Note: Bot checks every minute for scheduled times

### Redis connection errors
- Ensure Redis server is running: `redis-cli ping`
- Check Redis URL and password in `.env`
- Verify Redis port is not blocked by firewall
- For Docker: ensure container is accessible

### Tests failing
- Run `go mod download` to ensure dependencies
- Check Go version is 1.23+
- Run with `-v` flag for verbose output
- Tests use miniredis, so external Redis not needed


## License

MIT License - Feel free to use and modify!