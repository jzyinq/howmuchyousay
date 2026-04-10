package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractProductsFromPage_WithOGProduct(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/product/1",
		Markdown: "# Cool Product\n\nSome description of the product.\n\nPrice: 4999.00 PLN",
		Metadata: map[string]string{
			"ogTitle": "Cool Product",
			"ogImage": "https://shop.com/img/product1.jpg",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	require.Len(t, products, 1)
	assert.Equal(t, "Cool Product", products[0].Name)
	assert.Equal(t, 4999.00, products[0].Price)
	assert.Equal(t, "https://shop.com/img/product1.jpg", products[0].ImageURL)
	assert.Equal(t, "https://shop.com/product/1", products[0].SourceURL)
}

func TestExtractProductsFromPage_MultiplePricesReturnsNil(t *testing.T) {
	// Multiple prices in markdown — too ambiguous for metadata extraction
	page := crawler.PageResult{
		URL:      "https://shop.com/product/2",
		Markdown: "# Product A - 299.99 PLN\n\n# Product B - 499.99 PLN",
		Metadata: map[string]string{
			"ogTitle": "Products",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	assert.Empty(t, products)
}

func TestExtractProductsFromPage_NoMetadata(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/page",
		Markdown: "# Some page content",
		Metadata: map[string]string{},
	}

	products := crawler.ExtractProductsFromPage(page)
	assert.Empty(t, products)
}

func TestExtractProductsFromPage_EmptyMarkdown(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/empty",
		Markdown: "",
		Metadata: map[string]string{},
	}

	products := crawler.ExtractProductsFromPage(page)
	assert.Empty(t, products)
}

func TestExtractProductsFromPage_FallbackToTitle(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/product/3",
		Markdown: "# Product page\n\nOnly 199.99 for this item",
		Metadata: map[string]string{
			"title": "Fallback Title Product",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	require.Len(t, products, 1)
	assert.Equal(t, "Fallback Title Product", products[0].Name)
	assert.Equal(t, 199.99, products[0].Price)
}

func TestExtractProductsFromPage_CommaSeparatedPrice(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.pl/product/4",
		Markdown: "# Polish Product\n\nCena: 1499,99 zł",
		Metadata: map[string]string{
			"ogTitle": "Polish Product",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	require.Len(t, products, 1)
	assert.Equal(t, 1499.99, products[0].Price)
}

func TestExtractProductsFromPage_NoPriceInMarkdown(t *testing.T) {
	page := crawler.PageResult{
		URL:      "https://shop.com/article",
		Markdown: "# Some Article\n\nLong article content without any prices.",
		Metadata: map[string]string{
			"ogTitle": "Some Article",
		},
	}

	products := crawler.ExtractProductsFromPage(page)
	assert.Empty(t, products)
}

func TestHasProductSignals_WithProductKeywords(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     bool
	}{
		{"add to cart", "Click to add to cart now", true},
		{"buy now", "Buy now and save!", true},
		{"Polish add to cart", "Dodaj do koszyka aby kupić", true},
		{"price keyword", "Current price: $50", true},
		{"zł currency", "Cena: 100 zł", true},
		{"PLN currency", "Koszt: 200 PLN", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := crawler.PageResult{
				Markdown: tt.markdown,
				Metadata: map[string]string{},
			}
			assert.True(t, crawler.HasProductSignals(page))
		})
	}
}

func TestHasProductSignals_WithoutProductKeywords(t *testing.T) {
	page := crawler.PageResult{
		Markdown: "# About Us\n\nWe are a company that does things.",
		Metadata: map[string]string{
			"title": "About Us - Company",
		},
	}

	assert.False(t, crawler.HasProductSignals(page))
}

func TestHasProductSignals_WithPricePattern(t *testing.T) {
	page := crawler.PageResult{
		Markdown: "This item costs 4999,99 and is available now.",
		Metadata: map[string]string{},
	}

	assert.True(t, crawler.HasProductSignals(page))
}

func TestHasProductSignals_WithDotPricePattern(t *testing.T) {
	page := crawler.PageResult{
		Markdown: "Special offer: 199.99 only today",
		Metadata: map[string]string{},
	}

	assert.True(t, crawler.HasProductSignals(page))
}

func TestHasProductSignals_KeywordInTitle(t *testing.T) {
	page := crawler.PageResult{
		Markdown: "Some generic content without keywords",
		Metadata: map[string]string{
			"title": "Buy Now - Best Deals",
		},
	}

	assert.True(t, crawler.HasProductSignals(page))
}

func TestPageHasContent(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   \n\t\n   ", false},
		{"short content", "Hello", false},
		{"exactly 50 chars", "01234567890123456789012345678901234567890123456789", false},
		{"51 chars", "012345678901234567890123456789012345678901234567890", true},
		{"long content", "This is a page with enough content to be considered valid for processing.", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := crawler.PageResult{
				Markdown: tt.markdown,
				Metadata: map[string]string{},
			}
			assert.Equal(t, tt.want, crawler.PageHasContent(page))
		})
	}
}
