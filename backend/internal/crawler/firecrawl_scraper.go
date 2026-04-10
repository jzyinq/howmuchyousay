package crawler

import (
	"context"
	"fmt"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
)

// LinkDiscoveryResult contains the links and metadata from a page exploration.
type LinkDiscoveryResult struct {
	PageURL   string
	PageTitle string
	Links     []string
}

// ProductExtractionResult contains structured product data extracted by Firecrawl's LLM.
type ProductExtractionResult struct {
	Name     string
	Price    float64
	ImageURL string
	Found    bool
}

// FirecrawlScraper defines the interface for single-page scraping via Firecrawl.
type FirecrawlScraper interface {
	DiscoverLinks(ctx context.Context, url string) (*LinkDiscoveryResult, error)
	ExtractProduct(ctx context.Context, url string) (*ProductExtractionResult, error)
}

// FirecrawlClient implements FirecrawlScraper using the firecrawl-go SDK.
type FirecrawlClient struct {
	app *firecrawl.FirecrawlApp
}

// NewFirecrawlClient creates a new FirecrawlClient with the given API key and URL.
func NewFirecrawlClient(apiKey, apiURL string) (*FirecrawlClient, error) {
	app, err := firecrawl.NewFirecrawlApp(apiKey, apiURL)
	if err != nil {
		return nil, fmt.Errorf("creating firecrawl app: %w", err)
	}
	return &FirecrawlClient{app: app}, nil
}

// DiscoverLinks scrapes a page and returns all links found on it.
func (fc *FirecrawlClient) DiscoverLinks(ctx context.Context, url string) (*LinkDiscoveryResult, error) {
	onlyMainContent := true
	params := &firecrawl.ScrapeParams{
		Formats:         []string{"links"},
		OnlyMainContent: &onlyMainContent,
	}

	doc, err := fc.app.ScrapeURL(url, params)
	if err != nil {
		return nil, fmt.Errorf("firecrawl scrape (links): %w", err)
	}

	return convertLinksResponse(doc), nil
}

// ExtractProduct scrapes a product page and extracts structured data via Firecrawl's LLM.
func (fc *FirecrawlClient) ExtractProduct(ctx context.Context, url string) (*ProductExtractionResult, error) {
	prompt := "Extract the product name, price (as a number), and main product image URL from this product page."
	params := &firecrawl.ScrapeParams{
		Formats: []string{"json"},
		JsonOptions: &firecrawl.JsonOptions{
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string"},
					"price":     map[string]any{"type": "number"},
					"image_url": map[string]any{"type": "string"},
				},
				"required": []string{"name", "price"},
			},
			Prompt: &prompt,
		},
	}

	doc, err := fc.app.ScrapeURL(url, params)
	if err != nil {
		return nil, fmt.Errorf("firecrawl scrape (json): %w", err)
	}

	result := convertProductResponse(doc)
	return result, nil
}

// convertLinksResponse transforms a Firecrawl document into a LinkDiscoveryResult.
func convertLinksResponse(doc *firecrawl.FirecrawlDocument) *LinkDiscoveryResult {
	if doc == nil {
		return &LinkDiscoveryResult{}
	}

	result := &LinkDiscoveryResult{
		Links: doc.Links,
	}

	if doc.Metadata != nil {
		if doc.Metadata.Title != nil {
			result.PageTitle = *doc.Metadata.Title
		}
		if doc.Metadata.SourceURL != nil {
			result.PageURL = *doc.Metadata.SourceURL
		} else if doc.Metadata.URL != nil {
			result.PageURL = *doc.Metadata.URL
		}
	}

	return result
}

// convertProductResponse transforms a Firecrawl document into a ProductExtractionResult.
func convertProductResponse(doc *firecrawl.FirecrawlDocument) *ProductExtractionResult {
	if doc == nil || doc.JSON == nil || len(doc.JSON) == 0 {
		return &ProductExtractionResult{Found: false}
	}

	name, _ := doc.JSON["name"].(string)
	price, _ := doc.JSON["price"].(float64)
	imageURL, _ := doc.JSON["image_url"].(string)

	if name == "" || price == 0 {
		return &ProductExtractionResult{Found: false}
	}

	return &ProductExtractionResult{
		Name:     name,
		Price:    price,
		ImageURL: imageURL,
		Found:    true,
	}
}
