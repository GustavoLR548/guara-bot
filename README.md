# Godot News Bot

A Discord bot that automatically fetches Godot Engine news and posts AI-generated summaries in Brazilian Portuguese.

## Features

- 🤖 Automatic RSS monitoring (15min intervals)
- 🧠 AI summaries with Google Gemini 2.5 Flash
- 🇧🇷 Brazilian Portuguese summaries
- 📢 Multi-channel support (up to 5 per server)
- 🔒 Admin-only channel management
- 💾 Redis storage with pending queue
- ✅ Fully tested with TDD architecture

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
CHECK_INTERVAL_MINUTES=15
REDIS_URL=localhost:6379
REDIS_PASSWORD=
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
- Check RSS feed is accessible: `https://godotengine.org/rss.xml`
- Verify Gemini API key is valid
- Check logs for error messages

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

## Credits

Built with ❤️ for the Godot Engine community
