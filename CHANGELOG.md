# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2024-01-XX

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

## [0.3.0] - 2024-01-XX

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

## [0.2.0] - 2024-01-XX

### Added
- **Permission System**: All commands require "Manage Server" permission
- **Channel Limits**: Configurable maximum channels (default: 5)
- **Article History**: GUID-based deduplication
- **Portuguese Summaries**: AI generates summaries in PT-BR
- **15-minute Polling**: Automatic RSS checking

### Changed
- **Commands**: Added `/list-channels` for admins
- **AI Configuration**: Temperature 0.7, TopP 0.95, TopK 40

## [0.1.0] - 2024-01-XX

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
