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
/update-news [feed]             # Force check specific RSS feed (defaults to godot-official)
/update-all-news                # Force check all RSS feeds immediately
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

### Help & Information

```bash
/help  # Display all available commands organized by category
```

### GitHub Repository Monitoring

```bash
/register-repo <id> <owner> <repo> [branch]    # Register GitHub repository
/unregister-repo <id>                           # Remove repository
/list-repos                                     # Show all registered repos with stats
/setup-repo-channel #channel <id>              # Subscribe channel (use repo ID)
/remove-repo-channel #channel <id>             # Unsubscribe channel (use repo ID)
/schedule-repo <id> <times>                    # Set check times (use repo ID)
/update-repo <id>                              # Force check specific repository
/update-all-repos                              # Force check all repositories
```

**GitHub Examples:**

```bash
# Step 1: Register a repository with a custom ID
/register-repo godot-engine godotengine godot main
#              └─ Your ID  └─ Owner   └─ Repo └─ Branch

# Step 2: Subscribe a channel (use the same ID)
/setup-repo-channel #updates godot-engine
#                            └─ Same ID from registration
#   → If PRs were already detected, they'll be posted immediately!
#   → Bot processes 5 PRs per batch

# Step 3: Set schedule (use the same ID)
/schedule-repo godot-engine 09:00,13:00,18:00
#              └─ Same ID  └─ Check at 9 AM, 1 PM, 6 PM

# List all repos with stats
/list-repos

# Force check for updates (processes one batch if PRs are pending)
/update-repo godot-engine
#   → Checks GitHub for new PRs AND processes pending queue
#   → Processes max 5 PRs per call

# More examples with different repos
/register-repo rust-lang rust-lang rust master
/setup-repo-channel #rust-updates rust-lang
/schedule-repo rust-lang 10:00,16:00
```

**Batch Processing Behavior:**

- Bot fetches PRs from **last 3 days** (older PRs are automatically ignored)
- Processes maximum **5 PRs per batch** to stay within AI token limits
- **One batch at a time**: Bot processes 5 PRs, then waits for next scheduled check
- Schedule-based: If you have 42 PRs pending, bot will process them gradually:
  - First check: 5 PRs posted (37 remaining)
  - Next check: 5 PRs posted (32 remaining)
  - Continues until queue is empty
- `/update-repo` command: Manually trigger one batch processing
- Example: 42 pending PRs with 3 scheduled times daily = ~3 days to clear queue

**High-Value PR Filtering:**

- **Whitelist labels**: bug, enhancement, performance, optimization, usability, accessibility, security
- **Minimum line changes**: 5 (configurable via `GITHUB_FILTER_MIN_CHANGES`)
- **Auto-rejection**: Documentation-only, trivial changes, unlabeled minor PRs
- **Example logs in console**:
  ```
  [GITHUB-CLIENT] ✅ PR #114978 accepted (label: bug, changes: 14 lines)
  [GITHUB-CLIENT] ❌ PR #114979 rejected (too few changes: 1 < 5)
  [GITHUB-MONITOR] Total fetched: 2, Already processed: 0, Filtered out: 1, Accepted: 1
  ```

**Auto-Categorization:**

- **Features**: New capabilities and enhancements
- **Bugfixes**: Bug fixes and corrections
- **Performance**: Optimizations and speed improvements
- **UI/UX**: Interface and usability improvements
- **Security**: Security patches and vulnerability fixes

**Note:** PRs detected before any channel is registered are kept in a pending queue. When you register a channel with `/setup-repo-channel`, pending PRs will be processed gradually according to schedule!

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

# GitHub repositories
redis-cli KEYS "github:repos:*"                   # List all repos
redis-cli HGETALL github:repos:REPO_ID            # Repo details
redis-cli LRANGE github:repos:REPO_ID:schedule 0 -1  # Repo schedule
redis-cli LRANGE github:repos:REPO_ID:pending 0 -1   # Pending PRs
redis-cli SMEMBERS github:repos:REPO_ID:channels     # Subscribed channels
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

# GitHub Integration (Optional)
GITHUB_TOKEN=                     # GitHub Personal Access Token
GITHUB_CHECK_INTERVAL_MINUTES=30  # Fallback for repos without schedules
GITHUB_BATCH_THRESHOLD=5          # PRs needed to trigger summary
GITHUB_FILTER_MIN_CHANGES=5       # Minimum line changes

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
