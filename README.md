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

### Discord Setup

**Bot Permissions:** `277025508416`

**Invite URL:**
```
https://discord.com/api/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=277025508416&scope=bot%20applications.commands
```

## Commands

| Command | Description | Permission |
|---------|-------------|------------|
| `/setup-news #channel` | Register a channel for news | Manage Server |
| `/remove-news #channel` | Remove a channel | Manage Server |
| `/list-channels` | Show registered channels | Manage Server |
| `/update-news` | Force news check | Manage Server |

## Architecture

```
cmd/bot/main.go              # Entry point
internal/
  ├── ai/gemini.go          # AI summarization
  ├── bot/                   # Bot logic + commands
  ├── news/fetcher.go       # RSS + HTML scraping
  └── storage/repository.go  # Redis persistence
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o godot-news-bot ./cmd/bot

# Run locally
./godot-news-bot
```

## Tech Stack

- Go 1.23+
- Discord: discordgo v0.28.1
- Redis: go-redis v9.17.2
- AI: Google Gemini 2.5 Flash
- RSS: gofeed v1.3.0
- Testing: testify + miniredis

## Documentation

- [QUICKSTART.md](QUICKSTART.md) - Quick start guide
- [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md) - Project overview
- [CHANGELOG.md](CHANGELOG.md) - Version history

## License

MIT License - See LICENSE file for details

- **Testing**: [testify](https://github.com/stretchr/testify) + [miniredis](https://github.com/alicebob/miniredis)

## Setup

### 1. Install Go 1.23+

```bash
go version  # Should be 1.23 or higher
```

### 2. Install Redis

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install redis-server
sudo systemctl start redis-server
sudo systemctl enable redis-server
```

**macOS:**
```bash
brew install redis
brew services start redis
```

**Docker:**
```bash
docker run -d -p 6379:6379 redis:7-alpine
```

Verify Redis is running:
```bash
redis-cli ping  # Should return PONG
```

### 3. Clone and Install Dependencies

```bash
cd /home/lopin/guara-bot
go mod download
```

### 4. Configure Environment Variables

Create a `.env` file with the following variables:

```env
DISCORD_TOKEN=your_discord_bot_token
GEMINI_API_KEY=your_gemini_api_key
MAX_CHANNELS_LIMIT=5
CHECK_INTERVAL_MINUTES=15
REDIS_URL=localhost:6379
REDIS_PASSWORD=
```

**Environment Variables:**
- `DISCORD_TOKEN`: Your Discord bot token (required)
- `GEMINI_API_KEY`: Your Google Gemini API key (required)
- `MAX_CHANNELS_LIMIT`: Maximum number of channels to broadcast news (default: 5)
- `CHECK_INTERVAL_MINUTES`: Interval in minutes to check for new articles (default: 15)
- `REDIS_URL`: Redis server address (default: localhost:6379)
- `REDIS_PASSWORD`: Redis password if authentication is enabled (optional)

### 5. Run Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/storage/
go test ./internal/news/
go test ./internal/ai/
go test ./internal/bot/
```

**Note:** Tests use miniredis (in-memory Redis mock), so no external Redis server is needed for testing.

### 6. Build the Bot

```bash
go build -o godot-news-bot ./cmd/bot
```

### 7. Run the Bot

```bash
./godot-news-bot
```

Or run directly:

```bash
go run ./cmd/bot
```

## Usage

### Discord Commands

#### `/setup-news`
- **Permission Required**: Manage Server
- **Description**: Registers the current channel to receive news updates
- **Constraints**: Respects `MAX_CHANNELS_LIMIT` from `.env`

#### `/remove-news`
- **Permission Required**: Manage Server
- **Description**: Removes the current channel from receiving news updates
- **Effect**: Channel will no longer receive Godot news posts

#### `/list-channels`
- **Permission Required**: Manage Server
- **Description**: Lists all channels currently registered for news updates
- **Output**: Shows channel names and IDs, along with current count vs limit

### How It Works

1. **News Loop** (Configurable interval via `CHECK_INTERVAL_MINUTES`):
   - Fetches RSS from `https://godotengine.org/rss.xml`
   - Checks if the latest article is new (via GUID)
   - Scrapes full article content
   - Generates AI summary in PT-BR
   - Broadcasts to all registered channels

2. **Channel Management**:
   - Channels are stored in `channels.json`
   - Limit enforced via `MAX_CHANNELS_LIMIT`
   - Only users with "Manage Server" permission can add channels

3. **History Tracking**:
   - Article GUIDs stored in `history.json`
   - Prevents duplicate posts

## Testing

All core logic is fully testable thanks to interface-based design:

### Interface Implementations

```go
// Storage
type ChannelRepository interface { ... }
type HistoryRepository interface { ... }

// News
type NewsFetcher interface { ... }

// AI
type AISummarizer interface { ... }
```

### Mock Usage Example

```go
// Create mock for testing
mock := &ai.MockAISummarizer{
    SummarizeFunc: func(ctx context.Context, text string) (string, error) {
        return "TL;DR: Test summary in PT-BR", nil
    },
}

// Use in tests
summary, err := mock.Summarize(ctx, "article text")
```

## Development

### Adding New Commands

1. Add command to [internal/bot/commands.go](internal/bot/commands.go)
2. Register in `RegisterCommands()` function
3. Add handler in `HandleCommands()` switch statement
4. Write tests in [internal/bot/commands_test.go](internal/bot/commands_test.go)

### Adding New Features

1. Define interface first (e.g., `type MyFeature interface { ... }`)
2. Create implementation
3. Write comprehensive tests (table-driven preferred)
4. Inject via `main.go` dependency injection

## Project Structure Highlights

### Dependency Injection
All dependencies are injected through constructors:
- `NewBot()` receives all interfaces
- `NewCommandHandler()` receives repository
- Easy to swap implementations for testing

### Table-Driven Tests
All tests use Go's idiomatic table-driven pattern:
```go
tests := []struct {
    name     string
    input    string
    expected string
}{
    // test cases...
}
```

### Thread Safety
- Redis handles concurrency natively
- Safe for production use with multiple instances

## Redis Data Structure

The bot uses the following Redis keys:

- `news:channels` - SET: All registered Discord channel IDs
- `news:history:{guid}` - STRING: Individual article history (90-day TTL)
- `news:last_guid` - STRING: Most recently posted article GUID
- `news:config:max_channels` - STRING: Maximum channel limit configuration

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCORD_TOKEN` | (required) | Discord bot token |
| `GEMINI_API_KEY` | (required) | Google Gemini API key |
| `MAX_CHANNELS_LIMIT` | 5 | Maximum channels for news posting |
| `CHECK_INTERVAL_MINUTES` | 15 | How often to check for news (in minutes) |
| `REDIS_URL` | localhost:6379 | Redis server address |
| `REDIS_PASSWORD` | (empty) | Redis authentication password (optional) |

## License

MIT License - Feel free to use and modify!

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

## Credits

Built with ❤️ for the Godot Engine community
