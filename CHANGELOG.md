# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - TBD

### Changed
- **Rebranded to "Guara Bot"**: Bot is no longer Godot-specific
  - Updated all documentation to reflect general-purpose RSS news aggregation
  - Bot now supports any RSS feed source (blogs, news sites, game engines, tech sites, etc.)
  - Godot Engine feed remains as default example but can be replaced
  - Updated README, QUICKSTART, and PROJECT_SUMMARY with diverse feed examples
- **Command Updates**: Improved update commands for better feed control
  - `/update-news [feed]` - Now accepts optional feed parameter (defaults to godot-official)
  - `/update-all-news` - New command to force check all registered feeds
  - Better separation between single-feed and all-feeds updates

## [1.2.0] - TBD

### 🔴 BREAKING CHANGES
- **Redis Schema Changed**: Channel-feed relationship is now many-to-many
  - Channels can subscribe to multiple feeds
  - Each feed can have multiple channels
  - Migration required for existing deployments (see Migration Guide below)
- **Command Signatures**: `/setup-news` and `/remove-news` now require feed identifiers
  - `/setup-news #channel [feed]` - feed defaults to "godot-official"
  - `/remove-news #channel [feed]` - feed required if channel has multiple subscriptions

### Added
- **Multiple RSS Feed Support**: Register and manage multiple news feeds
  - `/register-feed <identifier> <url> [title] [description]` - Register new RSS feed
  - `/unregister-feed <identifier>` - Remove RSS feed
  - `/list-feeds` - Show all registered feeds with schedules and subscriber counts
  - Feed repository with full CRUD operations
  - Default "godot-official" feed created automatically for backward compatibility
- **Time-Based Scheduling**: Configure specific check times instead of intervals
  - `/schedule-feed <identifier> <times>` - Set check times (e.g., "09:00,13:00,18:00")
  - 24-hour format validation (HH:MM)
  - Per-feed schedules stored in Redis
  - Empty schedule falls back to interval-based checking
- **Feed Management Architecture**:
  - New `FeedRepository` interface with Redis implementation
  - Updated `ChannelRepository` for many-to-many relationships
  - Redis keys: `news:feeds:{id}`, `news:channels:{id}:feeds`, `news:feeds:{id}:schedule`
  - Feed struct with ID, URL, Title, Description, AddedAt, Schedule fields

### Changed
- **Channel Operations**: Now require feed identifier parameter
  - `AddChannel(channelID, feedID)` - Associate channel with specific feed
  - `RemoveChannel(channelID, feedID)` - Remove specific feed association
  - `GetChannelFeeds(channelID)` - List all feeds for a channel
  - `GetFeedChannels(feedID)` - List all channels for a feed
- **Command Behavior**:
  - `/setup-news` accepts optional feed parameter (defaults to godot-official)
  - `/remove-news` requires feed if channel has multiple subscriptions
  - `/list-channels` shows feed associations per channel
- **Storage Layer**: Complete refactor for multi-feed support
  - 14 comprehensive tests for feed repository
  - Schedule validation in repository layer
  - Atomic operations with Redis pipelines

### Technical Details
- **New Tests**: 14 feed repository tests + 11 updated channel tests
  - Feed registration, unregistration, CRUD operations
  - Schedule management with validation
  - Channel-feed association tests
  - Time format validation (HH:MM)
- **Redis Operations**: Optimized with pipelining
  - Feed data in HASH (url, title, description, added_at)
  - Channel-feed associations in SET
  - Schedules in LIST with validated times
- **Validation**: Time format strictly validated (HH:MM, 00:00-23:59)
- **Backward Compatibility**: Default feed auto-created on startup

### Migration Guide
For existing deployments upgrading from v1.1.0:

1. **Backup Redis Data**:
   ```bash
   redis-cli SAVE
   cp /var/lib/redis/dump.rdb /backup/dump.rdb.v1.1.0
   ```

2. **Update Environment** (optional - removes old interval config):
   ```bash
   # Remove or keep CHECK_INTERVAL_MINUTES (still supported as fallback)
   # Add per-feed schedules via /schedule-feed command after upgrade
   ```

3. **Deploy v1.2.0**:
   - Bot will auto-create "godot-official" feed
   - Existing channel associations will need to be re-created
   - **Important**: All channels must be re-registered with `/setup-news #channel godot-official`

4. **Re-register Channels**:
   ```
   /setup-news #news-channel godot-official
   ```

5. **Optional - Set Schedules**:
   ```
   /schedule-feed godot-official 09:00,13:00,18:00
   ```

### Deprecation Notice
- `CHECK_INTERVAL_MINUTES` will be removed in v2.0.0 (use per-feed schedules instead)

## [1.1.0] - 01/01/2026

### Added
- **Cost Management System**: Comprehensive rate limiting to prevent exceeding Gemini API free tier
  - Token counting before requests (input + output estimation)
  - Request rate limiting (10 RPM default, configurable)
  - Token rate limiting (200k TPM default, configurable)
  - Per-request token limit (4k default, configurable)
- **Circuit Breaker Pattern**: Automatically stops requests after 5 consecutive failures
  - 5-minute timeout before retry (configurable)
  - Prevents cascade failures during API outages
- **Retry Logic**: Exponential backoff for failed requests
  - Up to 3 retry attempts (configurable)
  - Backoff: 1s, 2s, 4s, 8s... (capped at 60s)
  - Smart retry detection (retries 429/503, skips 400/401/403)
- **Rate Limit Statistics**: Expose current usage metrics
  - Current window requests/tokens
  - Total requests/tokens/failures
  - Circuit breaker status
- **Environment Configuration**: All rate limits configurable via .env
  - `GEMINI_MAX_REQUESTS_PER_MINUTE`
  - `GEMINI_MAX_TOKENS_PER_MINUTE`
  - `GEMINI_MAX_TOKENS_PER_REQUEST`
  - `GEMINI_CIRCUIT_BREAKER_THRESHOLD`
  - `GEMINI_CIRCUIT_BREAKER_TIMEOUT_MINUTES`
  - `GEMINI_RETRY_ATTEMPTS`
  - `GEMINI_RETRY_BACKOFF_SECONDS`
- **Comprehensive Tests**: 16 new tests for rate limiting system
  - Token estimation tests
  - Circuit breaker tests
  - Concurrent access tests
  - Window reset tests
  - Retry logic tests

### Changed
- **AI Summarization**: Now wrapped with rate limiting and retry logic
- **Token Counting**: Automatic token estimation before API calls
- **Error Handling**: Improved with retry logic and circuit breaker
- **Logging**: Enhanced with token usage and rate limit information

### Technical Details
- New package: `internal/ratelimit` with 16 passing tests
- Thread-safe rate limiting with mutex protection
- Conservative defaults (10 RPM, 200k TPM) well below free tier limits (15 RPM, 250k TPM)
- Wait-for-capacity mechanism to handle bursts gracefully

## [1.0.0] - 31/12/2025

### Added
- **Channel Parameters**: Commands now accept channel hashtag parameters (`/setup-news #channel`)
- **Pending Queue**: Up to 5 articles queued when no channels are registered
- **Enhanced Logging**: Comprehensive AI API logging (input length, timing, finish reason, punctuation validation)
- **Validation**: `update-news` command requires at least 1 registered channel
- **Docker Deployment**: Multi-stage build producing ~15MB image
- **Redis Persistence**: AOF persistence with health checks in docker-compose

### Changed
- **AI Model**: Upgraded from gemini-pro to gemini-2.5-flash
- **AI Timeout**: Increased from 2 minutes to 5 minutes
- **Max Tokens**: Increased from 1000 to 1500 for longer summaries
- **AI Prompt**: Added explicit instruction to avoid preambles ("Aqui está um resumo...")
- **List Channels**: Simplified to show channel mentions only (no Discord API calls)
- **Architecture**: Reordered logic to check channels BEFORE generating summaries

### Fixed
- Gemini API 404 errors with invalid model names
- Bot generating summaries with zero channels registered (wasted API quota)
- Summaries cutting off mid-sentence
- "Integração desconhecida" error in list-channels command
- Summary preambles appearing in output

## [0.3.0] - 31/12/2025

### Added
- **Redis Storage**: Migrated from JSON files to Redis
- **Pending Queue**: Article queue for channels-not-registered scenarios
- **Miniredis Testing**: In-memory Redis mock for isolated tests
- **TTL Support**: 90-day automatic history cleanup

### Changed
- **Repository Interfaces**: Expanded with pending queue methods
- **Storage Layer**: Full Redis implementation with atomic operations

### Removed
- JSON file storage (`news_data.json`)

## [0.2.0] - 31/12/2025

### Added
- **Permission System**: All commands require "Manage Server" permission
- **Channel Limits**: Configurable maximum channels (default: 5)
- **Article History**: GUID-based deduplication
- **Portuguese Summaries**: AI generates summaries in PT-BR
- **15-minute Polling**: Automatic RSS checking

### Changed
- **Commands**: Added `/list-channels` for admins
- **AI Configuration**: Temperature 0.7, TopP 0.95, TopK 40

## [0.1.0] - 31/12/2025

### Added
- **Initial Project Scaffolding**: TDD architecture with Go 1.23
- **Discord Bot**: Basic slash commands (`/setup-news`, `/remove-news`, `/update-news`)
- **RSS Fetching**: Godot Engine news from godotengine.org/rss.xml
- **AI Summarization**: Google Gemini API integration
- **HTML Scraping**: go-readability for article content extraction
- **Test Suite**: 39 tests across all packages
- **Dependency Injection**: Full DI setup in main.go

### Technical Details
- **Interfaces**: NewsFetcher, AISummarizer, ChannelRepository, HistoryRepository
- **Mock Implementations**: For all external dependencies
- **Table-Driven Tests**: Comprehensive test scenarios

---

## Legend

- **Added**: New features
- **Changed**: Changes in existing functionality
- **Deprecated**: Soon-to-be removed features
- **Removed**: Removed features
- **Fixed**: Bug fixes
- **Security**: Vulnerability fixes

---

## Version History

- **v1.0.0**: Production-ready release with Redis, Docker, and enhanced UX
- **v0.3.0**: Redis migration with pending queue
- **v0.2.0**: Permission system and configuration options
- **v0.1.0**: Initial TDD scaffolding and core features
