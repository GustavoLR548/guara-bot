# Quick Start Guide

## Setup

1. **Install Redis:**
```bash
# Ubuntu/Debian
sudo apt install redis-server

# macOS
brew install redis
brew services start redis
```

2. **Configure bot:**
```bash
cp .env.example .env
# Edit .env with your tokens
```

3. **Run with Docker:**
```bash
docker-compose up -d
docker-compose logs -f bot
```

Or locally:
```bash
go run ./cmd/bot
```

## Discord Commands

### Channel Management
```bash
/setup-news #channel [feed]     # Subscribe channel to feed (defaults to godot-official)
/remove-news #channel [feed]    # Unsubscribe channel from feed
/list-channels                  # List all channels and subscriptions
/update-news [feed]             # Force check specific feed (defaults to godot-official)
/update-all-news                # Force check all feeds immediately
```

### Feed Management
```bash
/register-feed <id> <url> [title] [description]  # Register new RSS feed
/unregister-feed <id>                            # Remove RSS feed
/list-feeds                                       # Show all feeds (anyone can use)
/schedule-feed <id> <times>                      # Set check times (e.g., 09:00,13:00,18:00)
```

### Language Configuration
```bash
/set-language <language>                 # Set server default language
/set-channel-language #channel [language] # Override language for specific channel
```

**Supported Languages:**
- 🇧🇷 `pt-BR` - Português (Brasil)
- 🇺🇸 `en` - English
- 🇪🇸 `es` - Español
- 🇫🇷 `fr` - Français
- 🇩🇪 `de` - Deutsch
- 🇯🇵 `ja` - 日本語

**Examples:**
```bash
# Register feeds from different sources
/register-feed godot https://godotengine.org/rss.xml "Godot Engine" "Game engine news"
/register-feed techcrunch https://techcrunch.com/feed/ "TechCrunch" "Tech industry news"
/register-feed hackernews https://hnrss.org/frontpage "Hacker News" "Tech community"

# Subscribe channels to different feeds
/setup-news #game-dev godot
/setup-news #tech-news techcrunch
/setup-news #tech-news hackernews

# Set schedule for a feed (check at 9 AM, 1 PM, and 6 PM)
/schedule-feed godot 09:00,13:00,18:00

# Configure languages
/set-language en                        # Set entire server to English
/set-channel-language #brazilian pt-BR  # Portuguese for specific channel
/set-channel-language #spanish es       # Spanish for specific channel
/set-channel-language #german de        # German for specific channel
```

**Multi-Language Setup:**
```bash
# International community with language-specific channels
/set-language en                      # Server defaults to English
/set-channel-language #português pt-BR
/set-channel-language #español es
/set-channel-language #français fr
/set-channel-language #deutsch de
/set-channel-language #日本語 ja
```

**How Language Detection Works:**
1. Checks if channel has specific language override
2. Falls back to guild/server default language
3. Falls back to English (en) if nothing is set
4. Smart grouping: generates one summary per language, shared across channels

All commands (except `/list-feeds`) require **Manage Server** permission.

## Testing

```bash
# Run all tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/storage -v
```

## Redis Monitoring

```bash
# List all feeds
redis-cli KEYS "news:feeds:*"

# View feed details
redis-cli HGETALL news:feeds:godot-official

# Check feed schedule
redis-cli LRANGE news:feeds:godot-official:schedule 0 -1

# View channels subscribed to a feed
redis-cli SMEMBERS news:channels:CHANNEL_ID:feeds

# View last article for a feed
redis-cli GET news:history:godot-official:last

# View pending queue for a feed
redis-cli LRANGE news:history:godot-official:pending 0 -1

# Check all registered channels
redis-cli KEYS "news:channels:*:feeds"

# Language preferences
redis-cli GET news:guilds:GUILD_ID:language       # Guild default language
redis-cli GET news:channels:CHANNEL_ID:language   # Channel language override
```

## Docker Management

```bash
# View logs
docker-compose logs -f bot

# Restart bot
docker-compose restart bot

# Stop all
docker-compose down

# Rebuild
docker-compose up --build -d

# Clear all data from Redis
docker-compose exec redis redis-cli flushall
```

## Troubleshooting

**Bot not connecting:**
- Check `DISCORD_TOKEN` in `.env`
- Verify bot has permissions in Discord Developer Portal

**Redis connection failed:**
```bash
redis-cli ping  # Should return PONG
```

**No news posting:**
- Check `/list-channels` - at least 1 channel must be subscribed
- Check `/list-feeds` - verify feeds are registered
- Verify feed schedules with `/list-feeds` (or set with `/schedule-feed`)
- Use `/update-news` to force check all feeds
- View logs: `docker-compose logs bot`

**Feed not updating:**
- Check feed URL is accessible: `curl <feed-url>`
- Verify schedule is set: `/list-feeds`
- Check if it's the scheduled time (bot checks every minute)
- For immediate testing, feeds without schedules check every 15 minutes

## Environment Variables

```env
DISCORD_TOKEN=your_token           # Required
GEMINI_API_KEY=your_key           # Required
MAX_CHANNELS_LIMIT=5              # Optional (default: 5)
CHECK_INTERVAL_MINUTES=15         # Optional (fallback for feeds without schedules)
REDIS_URL=localhost:6379          # Optional
REDIS_PASSWORD=                   # Optional

# Rate Limiting (Optional - Gemini Free Tier Protection)
GEMINI_MAX_REQUESTS_PER_MINUTE=10
GEMINI_MAX_TOKENS_PER_MINUTE=200000
GEMINI_MAX_TOKENS_PER_REQUEST=4000
GEMINI_CIRCUIT_BREAKER_THRESHOLD=5
GEMINI_CIRCUIT_BREAKER_TIMEOUT_MINUTES=5
GEMINI_RETRY_ATTEMPTS=3
GEMINI_RETRY_BACKOFF_SECONDS=1
```

## Bot Invite URL

```
https://discord.com/api/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=277025508416&scope=bot%20applications.commands
```

Replace `YOUR_CLIENT_ID` with your bot's ID from Discord Developer Portal.

## Next Steps

- Setup automatic monitoring in your Discord server
- Configure Redis persistence for production
- Check [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md) for architecture details

