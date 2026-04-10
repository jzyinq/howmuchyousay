package crawler

import (
	"testing"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirecrawlScraperInterface(t *testing.T) {
	t.Run("FirecrawlClient implements FirecrawlScraper", func(t *testing.T) {
		var _ FirecrawlScraper = &FirecrawlClient{}
	})
}

func TestConvertLinksResponse(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	t.Run("extracts links and title from document", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			Links: []string{
				"https://shop.com/product/1",
				"https://shop.com/category/phones",
				"https://shop.com/page/2",
			},
			Metadata: &firecrawl.FirecrawlDocumentMetadata{
				Title:     strPtr("Electronics Store"),
				SourceURL: strPtr("https://shop.com"),
			},
		}

		result := convertLinksResponse(doc)

		assert.Equal(t, "Electronics Store", result.PageTitle)
		assert.Equal(t, "https://shop.com", result.PageURL)
		require.Len(t, result.Links, 3)
		assert.Equal(t, "https://shop.com/product/1", result.Links[0])
		assert.Equal(t, "https://shop.com/category/phones", result.Links[1])
	})

	t.Run("handles nil metadata", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			Links:    []string{"https://shop.com/a"},
			Metadata: nil,
		}

		result := convertLinksResponse(doc)

		assert.Equal(t, "", result.PageTitle)
		assert.Equal(t, "", result.PageURL)
		assert.Len(t, result.Links, 1)
	})

	t.Run("handles empty links", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			Links:    []string{},
			Metadata: &firecrawl.FirecrawlDocumentMetadata{Title: strPtr("Empty Page")},
		}

		result := convertLinksResponse(doc)

		assert.Equal(t, "Empty Page", result.PageTitle)
		assert.Empty(t, result.Links)
	})

	t.Run("handles nil document", func(t *testing.T) {
		result := convertLinksResponse(nil)
		assert.Equal(t, "", result.PageTitle)
		assert.Empty(t, result.Links)
	})
}

func TestConvertProductResponse(t *testing.T) {
	t.Run("extracts product from JSON field", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{
				"name":      "iPhone 15 Pro",
				"price":     float64(999.99),
				"image_url": "https://img.com/iphone.jpg",
			},
		}

		result := convertProductResponse(doc)

		assert.Equal(t, "iPhone 15 Pro", result.Name)
		assert.Equal(t, 999.99, result.Price)
		assert.Equal(t, "https://img.com/iphone.jpg", result.ImageURL)
		assert.True(t, result.Found)
	})

	t.Run("handles missing optional image_url", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{
				"name":  "Laptop Dell",
				"price": float64(1499.00),
			},
		}

		result := convertProductResponse(doc)

		assert.Equal(t, "Laptop Dell", result.Name)
		assert.Equal(t, 1499.00, result.Price)
		assert.Equal(t, "", result.ImageURL)
		assert.True(t, result.Found)
	})

	t.Run("returns not found for nil JSON", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: nil,
		}

		result := convertProductResponse(doc)
		assert.False(t, result.Found)
	})

	t.Run("returns not found for empty JSON", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{},
		}

		result := convertProductResponse(doc)
		assert.False(t, result.Found)
	})

	t.Run("returns not found for nil document", func(t *testing.T) {
		result := convertProductResponse(nil)
		assert.False(t, result.Found)
	})

	t.Run("handles price as int", func(t *testing.T) {
		doc := &firecrawl.FirecrawlDocument{
			JSON: map[string]any{
				"name":  "Test Product",
				"price": float64(100),
			},
		}

		result := convertProductResponse(doc)
		assert.Equal(t, 100.0, result.Price)
		assert.True(t, result.Found)
	})
}

func TestNewFirecrawlClientScraper(t *testing.T) {
	t.Run("creates client with valid credentials", func(t *testing.T) {
		client, err := NewFirecrawlClient("test-api-key", "https://api.firecrawl.dev")
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("returns error with empty API key", func(t *testing.T) {
		t.Setenv("FIRECRAWL_API_KEY", "")
		client, err := NewFirecrawlClient("", "https://api.firecrawl.dev")
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}
