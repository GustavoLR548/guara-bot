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

```bash
/setup-news #channel    # Register channel for news
/remove-news #channel   # Remove channel
/list-channels          # List all registered channels
/update-news            # Force check for news
```

All commands require **Manage Server** permission.

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
# Check channels
redis-cli SMEMBERS news:channels

# View last article
redis-cli GET news:last_guid

# View pending queue
redis-cli LRANGE news:pending_queue 0 -1
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
- Check `/list-channels` - at least 1 channel must be registered
- Use `/update-news` to force check
- View logs: `docker-compose logs bot`

## Environment Variables

```env
DISCORD_TOKEN=your_token           # Required
GEMINI_API_KEY=your_key           # Required
MAX_CHANNELS_LIMIT=5              # Optional (default: 5)
CHECK_INTERVAL_MINUTES=15         # Optional (default: 15)
REDIS_URL=localhost:6379          # Optional
REDIS_PASSWORD=                   # Optional
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

