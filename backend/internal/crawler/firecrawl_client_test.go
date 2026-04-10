package crawler

import (
	"context"
	"testing"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CrawlSite is tested indirectly through convertDocuments (which handles all
// response transformation logic) and through the FirecrawlCrawler interface
// mock (which verifies callers use the interface correctly). Direct unit
// testing of CrawlSite is not practical because the firecrawl-go SDK's
// internal HTTP client cannot be easily substituted. Full integration testing
// of CrawlSite requires a real Firecrawl API key and endpoint.

// mockFirecrawlCrawler is a test double for FirecrawlCrawler.
type mockFirecrawlCrawler struct {
	result []PageResult
	err    error
}

func (m *mockFirecrawlCrawler) CrawlSite(ctx context.Context, siteURL string, cfg CrawlConfig) ([]PageResult, error) {
	return m.result, m.err
}

func TestFirecrawlCrawlerInterface(t *testing.T) {
	t.Run("mock implements interface", func(t *testing.T) {
		var _ FirecrawlCrawler = &mockFirecrawlCrawler{}
	})

	t.Run("FirecrawlClient implements interface", func(t *testing.T) {
		var _ FirecrawlCrawler = &FirecrawlClient{}
	})
}

func TestConvertDocuments(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	t.Run("converts documents with full metadata", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "# Product Page\nSome content",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					Title:      strPtr("Product Page"),
					SourceURL:  strPtr("https://example.com/products/1"),
					StatusCode: intPtr(200),
					OGTitle:    &firecrawl.StringOrStringSlice{"OG Title"},
					OGImage:    &firecrawl.StringOrStringSlice{"https://example.com/img.jpg"},
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "https://example.com/products/1", results[0].URL)
		assert.Equal(t, "# Product Page\nSome content", results[0].Markdown)
		assert.Equal(t, "Product Page", results[0].Metadata["title"])
		assert.Equal(t, "https://example.com/products/1", results[0].Metadata["sourceURL"])
		assert.Equal(t, "200", results[0].Metadata["statusCode"])
		assert.Equal(t, "OG Title", results[0].Metadata["ogTitle"])
		assert.Equal(t, "https://example.com/img.jpg", results[0].Metadata["ogImage"])
	})

	t.Run("converts multiple documents", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "Page 1",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					SourceURL: strPtr("https://example.com/page1"),
				},
			},
			{
				Markdown: "Page 2",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					SourceURL: strPtr("https://example.com/page2"),
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 2)
		assert.Equal(t, "https://example.com/page1", results[0].URL)
		assert.Equal(t, "https://example.com/page2", results[1].URL)
	})

	t.Run("handles nil metadata", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "No metadata page",
				Metadata: nil,
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "", results[0].URL)
		assert.Equal(t, "No metadata page", results[0].Markdown)
		assert.NotNil(t, results[0].Metadata)
		assert.Empty(t, results[0].Metadata)
	})

	t.Run("handles nil document pointer in slice", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			nil,
			{
				Markdown: "Valid page",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					SourceURL: strPtr("https://example.com/valid"),
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "https://example.com/valid", results[0].URL)
	})

	t.Run("handles empty document slice", func(t *testing.T) {
		results := convertDocuments(nil)
		assert.Empty(t, results)

		results = convertDocuments([]*firecrawl.FirecrawlDocument{})
		assert.Empty(t, results)
	})

	t.Run("handles nil optional metadata fields", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "Sparse metadata",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					Title:      nil,
					SourceURL:  strPtr("https://example.com/sparse"),
					StatusCode: nil,
					OGTitle:    nil,
					OGImage:    nil,
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "https://example.com/sparse", results[0].URL)
		assert.Equal(t, "https://example.com/sparse", results[0].Metadata["sourceURL"])
		// Nil fields should not appear in metadata map
		_, hasTitle := results[0].Metadata["title"]
		assert.False(t, hasTitle)
		_, hasStatusCode := results[0].Metadata["statusCode"]
		assert.False(t, hasStatusCode)
		_, hasOGTitle := results[0].Metadata["ogTitle"]
		assert.False(t, hasOGTitle)
	})

	t.Run("uses metadata URL field as fallback when sourceURL is nil", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "URL from url field",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					URL:       strPtr("https://example.com/via-url"),
					SourceURL: nil,
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "https://example.com/via-url", results[0].URL)
	})

	t.Run("prefers sourceURL over URL field", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "Both URL fields",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					URL:       strPtr("https://example.com/url-field"),
					SourceURL: strPtr("https://example.com/source-url"),
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "https://example.com/source-url", results[0].URL)
	})

	t.Run("handles empty StringOrStringSlice", func(t *testing.T) {
		emptySlice := firecrawl.StringOrStringSlice{}
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "Empty string slice fields",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					SourceURL: strPtr("https://example.com/empty-slices"),
					OGTitle:   &emptySlice,
					OGImage:   &emptySlice,
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		_, hasOGTitle := results[0].Metadata["ogTitle"]
		assert.False(t, hasOGTitle)
	})

	t.Run("handles description StringOrStringSlice", func(t *testing.T) {
		docs := []*firecrawl.FirecrawlDocument{
			{
				Markdown: "With description",
				Metadata: &firecrawl.FirecrawlDocumentMetadata{
					SourceURL:   strPtr("https://example.com/desc"),
					Description: &firecrawl.StringOrStringSlice{"A great product page"},
				},
			},
		}

		results := convertDocuments(docs)

		require.Len(t, results, 1)
		assert.Equal(t, "A great product page", results[0].Metadata["description"])
	})
}

func TestNewFirecrawlClient(t *testing.T) {
	t.Run("creates client with valid credentials", func(t *testing.T) {
		client, err := NewFirecrawlClient("test-api-key", "https://api.firecrawl.dev")

		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("returns error with empty API key", func(t *testing.T) {
		// Unset env var to ensure SDK doesn't pick it up
		t.Setenv("FIRECRAWL_API_KEY", "")

		client, err := NewFirecrawlClient("", "https://api.firecrawl.dev")

		assert.Error(t, err)
		assert.Nil(t, client)
	})
}
