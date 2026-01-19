package news

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockNewsFetcher is a mock implementation of NewsFetcher for testing
type MockNewsFetcher struct {
	FetchLatestArticleFunc    func() (*Article, error)
	FetchArticlesFunc         func() ([]Article, error)
	ScrapeArticleContentFunc  func(url string) (string, error)
}

func (m *MockNewsFetcher) FetchLatestArticle() (*Article, error) {
	if m.FetchLatestArticleFunc != nil {
		return m.FetchLatestArticleFunc()
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *MockNewsFetcher) FetchArticles() ([]Article, error) {
	if m.FetchArticlesFunc != nil {
		return m.FetchArticlesFunc()
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *MockNewsFetcher) ScrapeArticleContent(url string) (string, error) {
	if m.ScrapeArticleContentFunc != nil {
		return m.ScrapeArticleContentFunc(url)
	}
	return "", fmt.Errorf("not implemented")
}

// TestRSSFetcher_FetchLatestArticle tests fetching articles from RSS
func TestRSSFetcher_FetchLatestArticle(t *testing.T) {
	tests := []struct {
		name          string
		rssContent    string
		expectError   bool
		errorContains string
		validateFunc  func(*testing.T, *Article)
	}{
		{
			name: "fetch valid RSS with single item",
			rssContent: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <guid>item-123</guid>
      <title>Test Article</title>
      <link>https://example.com/article</link>
      <description>Test description</description>
      <pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate>
    </item>
  </channel>
</rss>`,
			expectError: false,
			validateFunc: func(t *testing.T, article *Article) {
				assert.Equal(t, "item-123", article.GUID)
				assert.Equal(t, "Test Article", article.Title)
				assert.Equal(t, "https://example.com/article", article.Link)
				assert.Equal(t, "Test description", article.Description)
			},
		},
		{
			name: "fetch RSS with multiple items returns latest",
			rssContent: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <guid>latest-item</guid>
      <title>Latest Article</title>
      <link>https://example.com/latest</link>
    </item>
    <item>
      <guid>old-item</guid>
      <title>Old Article</title>
      <link>https://example.com/old</link>
    </item>
  </channel>
</rss>`,
			expectError: false,
			validateFunc: func(t *testing.T, article *Article) {
				assert.Equal(t, "latest-item", article.GUID)
				assert.Equal(t, "Latest Article", article.Title)
			},
		},
		{
			name: "error on empty RSS feed",
			rssContent: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Empty Feed</title>
  </channel>
</rss>`,
			expectError:   true,
			errorContains: "no articles found",
		},
		{
			name:          "error on invalid XML",
			rssContent:    `invalid xml content`,
			expectError:   true,
			errorContains: "failed to parse RSS feed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/rss+xml")
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(tt.rssContent)); err != nil {
					t.Errorf("Failed to write response: %v", err)
				}
			}))
			defer server.Close()

			fetcher := NewRSSFetcher(server.URL)
			article, err := fetcher.FetchLatestArticle()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, article)
				if tt.validateFunc != nil {
					tt.validateFunc(t, article)
				}
			}
		})
	}
}

// TestRSSFetcher_ScrapeArticleContent tests article content scraping
func TestRSSFetcher_ScrapeArticleContent(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		htmlContent   string
		statusCode    int
		expectError   bool
		errorContains string
		validateFunc  func(*testing.T, string)
	}{
		{
			name: "scrape valid HTML article",
			htmlContent: `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
  <article>
    <h1>Article Title</h1>
    <p>This is the first paragraph.</p>
    <p>This is the second paragraph.</p>
  </article>
</body>
</html>`,
			statusCode:  http.StatusOK,
			expectError: false,
			validateFunc: func(t *testing.T, content string) {
				assert.Contains(t, content, "Article Title")
				assert.Contains(t, content, "first paragraph")
				assert.Contains(t, content, "second paragraph")
			},
		},
		{
			name: "scrape article with extra whitespace",
			htmlContent: `<!DOCTYPE html>
<html>
<body>
  <article>
    <p>Line 1</p>


    <p>Line 2</p>
  </article>
</body>
</html>`,
			statusCode:  http.StatusOK,
			expectError: false,
			validateFunc: func(t *testing.T, content string) {
				// Should not have excessive newlines
				assert.NotContains(t, content, "\n\n\n")
			},
		},
		{
			name:          "error on 404 status",
			htmlContent:   "Not Found",
			statusCode:    http.StatusNotFound,
			expectError:   true,
			errorContains: "unexpected status code: 404",
		},
		{
			name:          "error on 500 status",
			htmlContent:   "Internal Server Error",
			statusCode:    http.StatusInternalServerError,
			expectError:   true,
			errorContains: "unexpected status code: 500",
		},
	}

	// Test empty URL separately
	t.Run("error on empty URL", func(t *testing.T) {
		fetcher := NewRSSFetcher("http://example.com")
		_, err := fetcher.ScrapeArticleContent("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty URL")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				}
				if _, err := w.Write([]byte(tt.htmlContent)); err != nil {
					t.Errorf("Failed to write response: %v", err)
				}
			}))
			defer server.Close()

			fetcher := NewRSSFetcher("http://example.com")
			content, err := fetcher.ScrapeArticleContent(server.URL)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, content)
				if tt.validateFunc != nil {
					tt.validateFunc(t, content)
				}
			}
		})
	}
}

// TestCleanText tests the text cleaning function
func TestCleanText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove extra newlines",
			input:    "Line 1\n\n\nLine 2\n\n\n\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "remove leading/trailing whitespace",
			input:    "  Line 1  \n  Line 2  ",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "handle CRLF line endings",
			input:    "Line 1\r\nLine 2\r\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \n  \n   ",
			expected: "",
		},
		{
			name:     "single line",
			input:    "Single line of text",
			expected: "Single line of text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsNewArticle tests article novelty checking
func TestIsNewArticle(t *testing.T) {
	tests := []struct {
		name        string
		currentGUID string
		lastGUID    string
		expected    bool
	}{
		{
			name:        "new article with different GUID",
			currentGUID: "guid-123",
			lastGUID:    "guid-456",
			expected:    true,
		},
		{
			name:        "same article",
			currentGUID: "guid-123",
			lastGUID:    "guid-123",
			expected:    false,
		},
		{
			name:        "first run with no history",
			currentGUID: "guid-123",
			lastGUID:    "",
			expected:    true,
		},
		{
			name:        "empty current GUID",
			currentGUID: "",
			lastGUID:    "guid-123",
			expected:    true,
		},
		{
			name:        "both empty",
			currentGUID: "",
			lastGUID:    "",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNewArticle(tt.currentGUID, tt.lastGUID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRSSFetcher_Timeout tests HTTP timeout behavior
func TestRSSFetcher_Timeout(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(35 * time.Second) // Longer than client timeout
		if _, err := w.Write([]byte("too slow")); err != nil {
			t.Logf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	fetcher := NewRSSFetcher(server.URL)
	
	// This should timeout
	_, err := fetcher.FetchLatestArticle()
	require.Error(t, err)
}

// TestRSSFetcher_Integration tests realistic RSS parsing
func TestRSSFetcher_Integration(t *testing.T) {
	// Realistic Godot-style RSS
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Godot Engine News</title>
    <link>https://godotengine.org</link>
    <description>Latest news from Godot Engine</description>
    <item>
      <guid>https://godotengine.org/article/godot-4-3-released</guid>
      <title>Godot 4.3 Released!</title>
      <link>https://godotengine.org/article/godot-4-3-released</link>
      <description>We are excited to announce the release of Godot 4.3</description>
      <pubDate>Mon, 01 Jan 2024 12:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	fetcher := NewRSSFetcher(server.URL)
	article, err := fetcher.FetchLatestArticle()

	require.NoError(t, err)
	assert.Equal(t, "https://godotengine.org/article/godot-4-3-released", article.GUID)
	assert.Equal(t, "Godot 4.3 Released!", article.Title)
	assert.Equal(t, "https://godotengine.org/article/godot-4-3-released", article.Link)
	assert.Contains(t, article.Description, "Godot 4.3")
}
