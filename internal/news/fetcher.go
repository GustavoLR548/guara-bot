package news

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
	"github.com/mmcdole/gofeed"
)

// Article represents a news article from the RSS feed
type Article struct {
	GUID        string
	Title       string
	Link        string
	Description string
	Content     string // Cleaned article content
	PublishDate time.Time
}

// NewsFetcher defines the interface for fetching and processing news
type NewsFetcher interface {
	// FetchLatestArticle fetches the most recent article from the RSS feed
	FetchLatestArticle() (*Article, error)
	// FetchArticles fetches all articles from the RSS feed
	FetchArticles() ([]Article, error)
	// ScrapeArticleContent fetches and cleans the full article content
	ScrapeArticleContent(url string) (string, error)
}

// RSSFetcher implements NewsFetcher using RSS feeds
type RSSFetcher struct {
	rssURL     string
	httpClient *http.Client
	parser     *gofeed.Parser
}

// NewRSSFetcher creates a new RSS-based news fetcher
func NewRSSFetcher(rssURL string) *RSSFetcher {
	return &RSSFetcher{
		rssURL: rssURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		parser: gofeed.NewParser(),
	}
}

// FetchLatestArticle fetches the most recent article from the RSS feed
func (f *RSSFetcher) FetchLatestArticle() (*Article, error) {
	feed, err := f.parser.ParseURL(f.rssURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	if len(feed.Items) == 0 {
		return nil, fmt.Errorf("no articles found in RSS feed")
	}

	// Get the first (most recent) item
	item := feed.Items[0]

	article := &Article{
		GUID:        item.GUID,
		Title:       item.Title,
		Link:        item.Link,
		Description: item.Description,
	}

	// Parse publish date
	if item.PublishedParsed != nil {
		article.PublishDate = *item.PublishedParsed
	} else if item.UpdatedParsed != nil {
		article.PublishDate = *item.UpdatedParsed
	} else {
		article.PublishDate = time.Now()
	}

	return article, nil
}

// FetchArticles fetches all articles from the RSS feed
func (f *RSSFetcher) FetchArticles() ([]Article, error) {
	feed, err := f.parser.ParseURL(f.rssURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	if len(feed.Items) == 0 {
		return nil, fmt.Errorf("no articles found in RSS feed")
	}

	articles := make([]Article, 0, len(feed.Items))
	for _, item := range feed.Items {
		article := Article{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
		}

		// Parse publish date
		if item.PublishedParsed != nil {
			article.PublishDate = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			article.PublishDate = *item.UpdatedParsed
		} else {
			article.PublishDate = time.Now()
		}

		articles = append(articles, article)
	}

	return articles, nil
}

// ScrapeArticleContent fetches and cleans the full article content
func (f *RSSFetcher) ScrapeArticleContent(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty URL provided")
	}

	// Fetch the article page
	resp, err := f.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch article: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse and clean using readability
	article, err := readability.FromReader(resp.Body, resp.Request.URL)
	if err != nil {
		return "", fmt.Errorf("failed to parse article: %w", err)
	}

	// Clean the text content
	content := cleanText(article.TextContent)

	if content == "" {
		return "", fmt.Errorf("no content extracted from article")
	}

	return content, nil
}

// cleanText removes extra whitespace and normalizes text
func cleanText(text string) string {
	// Remove excessive newlines
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	return strings.Join(cleaned, "\n")
}

// IsNewArticle checks if an article is new by comparing GUIDs
func IsNewArticle(currentGUID, lastGUID string) bool {
	if lastGUID == "" {
		return true // First run
	}
	return currentGUID != lastGUID
}
