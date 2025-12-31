# Project Summary

A Discord bot that monitors [Godot Engine news](https://godotengine.org/rss.xml) and posts AI-generated summaries to configured channels.

## Architecture

```
┌─────────────┐     ┌──────────┐     ┌─────────┐
│   Discord   │────▶│   Bot    │────▶│  Redis  │
│   Server    │◀────│ (Go 1.23)│◀────│ Storage │
└─────────────┘     └────┬─────┘     └─────────┘
                         │
                    ┌────▼────────┐
                    │   Gemini    │
                    │  AI (2.5)   │
                    └─────────────┘
```

### Components

**Bot Core** (`internal/bot/`)
- Slash command handlers with channel parameters
- 15-minute RSS polling (configurable)
- 5-article pending queue for zero-channel scenarios
- Permission validation (Manage Server required)

**Storage** (`internal/storage/`)
- Redis persistence with 5-second timeout
- Channel management (SET)
- Pending queue (LIST, max 5 articles)
- Article history (STRING with 90-day TTL)

**AI Summarization** (`internal/ai/`)
- Google Gemini 2.5 Flash
- 1500 max tokens, 5-minute timeout
- Enhanced logging (input length, API timing, finish reason)
- Portuguese output without preambles

**RSS Processor** (`internal/rss/`)
- gofeed v1.3.0 for parsing
- go-readability for HTML extraction
- GUID-based deduplication

## Tech Stack

- **Language**: Go 1.23+ with generics
- **Storage**: Redis 7 (go-redis v9.17.2)
- **AI**: Google Gemini 2.5 Flash
- **RSS**: gofeed v1.3.0, go-readability
- **Testing**: testify with miniredis v2.35.0
- **Deployment**: Docker multi-stage build (~15MB)

## Testing

**39 tests** across all packages with miniredis in-memory mocks:

```bash
go test ./...        # Run all tests
go test -cover ./... # With coverage
```

## Commands

| Command | Description | Permission |
|---------|-------------|------------|
| `/setup-news #channel` | Register channel | Manage Server |
| `/remove-news #channel` | Remove channel | Manage Server |
| `/list-channels` | List all channels | Manage Server |
| `/update-news` | Force news check | Manage Server |

## Configuration

```env
DISCORD_TOKEN=required
GEMINI_API_KEY=required
MAX_CHANNELS_LIMIT=5              # Optional
CHECK_INTERVAL_MINUTES=15         # Optional
REDIS_URL=localhost:6379          # Optional
REDIS_PASSWORD=                   # Optional
```

## Redis Schema

```
news:channels              → SET of channel IDs
news:pending_queue         → LIST of pending article GUIDs (max 5)
news:history:{guid}        → STRING "posted" (90-day TTL)
news:last_guid             → STRING of last processed GUID
```

## Development

**TDD Workflow:**
1. Write failing test
2. Implement minimal code to pass
3. Refactor while keeping tests green

**Dependencies:**
```bash
go mod download
```

**Run locally:**
```bash
go run ./cmd/bot
```

**Docker:**
```bash
docker-compose up --build
```

## Features

- ✅ Channel-based subscription with # parameters
- ✅ AI-powered summaries in Portuguese
- ✅ Pending queue (max 5 articles when no channels)
- ✅ Validation (update-news requires ≥1 channel)
- ✅ Docker deployment with Redis persistence
- ✅ 39 passing tests with full coverage
- ✅ Enhanced logging for debugging

## License

MIT License
