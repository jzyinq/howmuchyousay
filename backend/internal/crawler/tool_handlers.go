package crawler

import (
	"fmt"
	"strings"
)

// ToolHandlers manages crawl state for the orchestrator loop: visited URLs,
// saved products, scrape count, and deduplication.
type ToolHandlers struct {
	minProducts   int
	savedProducts []RawProduct
	seenNames     map[string]bool
	seenURLs      map[string]bool
	visitedURLs   map[string]bool
	scrapeCount   int
}

// NewToolHandlers creates a ToolHandlers with the given product target.
func NewToolHandlers(minProducts int) *ToolHandlers {
	return &ToolHandlers{
		minProducts: minProducts,
		seenNames:   make(map[string]bool),
		seenURLs:    make(map[string]bool),
		visitedURLs: make(map[string]bool),
	}
}

// SaveProduct validates and saves a product. Returns a status message for the orchestrator.
func (h *ToolHandlers) SaveProduct(p RawProduct) string {
	// Validate using existing validator (single-item slice)
	valid, rejected := ValidateProducts([]RawProduct{p})
	if len(rejected) > 0 {
		return fmt.Sprintf("Product rejected: %s. Not counted. Progress: %d/%d products.",
			rejected[0].Reason, len(h.savedProducts), h.minProducts)
	}
	if len(valid) == 0 {
		return fmt.Sprintf("Product rejected: validation failed. Not counted. Progress: %d/%d products.",
			len(h.savedProducts), h.minProducts)
	}

	p = valid[0]

	// Check duplicate by source URL
	if p.SourceURL != "" && h.seenURLs[p.SourceURL] {
		return fmt.Sprintf("Duplicate product (same URL) skipped. Progress: %d/%d products.",
			len(h.savedProducts), h.minProducts)
	}

	// Check duplicate by normalized name
	normalized := NormalizeName(p.Name)
	if h.seenNames[normalized] {
		return fmt.Sprintf("Duplicate product (same name) skipped. Progress: %d/%d products.",
			len(h.savedProducts), h.minProducts)
	}

	h.savedProducts = append(h.savedProducts, p)
	h.seenNames[normalized] = true
	if p.SourceURL != "" {
		h.seenURLs[p.SourceURL] = true
	}

	return fmt.Sprintf("Saved \"%s\" ($%.2f). Progress: %d/%d products.\nSaved so far: %s",
		p.Name, p.Price, len(h.savedProducts), h.minProducts, h.productNameList())
}

// GetStatus returns the current crawl progress.
func (h *ToolHandlers) GetStatus() string {
	if len(h.savedProducts) == 0 {
		return fmt.Sprintf("Progress: 0/%d products. No products saved yet.", h.minProducts)
	}
	return fmt.Sprintf("Progress: %d/%d products.\nSaved so far: %s",
		len(h.savedProducts), h.minProducts, h.productNameList())
}

// SavedProducts returns all saved products.
func (h *ToolHandlers) SavedProducts() []RawProduct {
	return h.savedProducts
}

// MinProducts returns the target product count.
func (h *ToolHandlers) MinProducts() int {
	return h.minProducts
}

// IsVisited checks if a URL has already been scraped.
func (h *ToolHandlers) IsVisited(url string) bool {
	return h.visitedURLs[url]
}

// MarkVisited records a URL as already scraped.
func (h *ToolHandlers) MarkVisited(url string) {
	h.visitedURLs[url] = true
}

// ScrapeCount returns the number of Firecrawl scrape calls made.
func (h *ToolHandlers) ScrapeCount() int {
	return h.scrapeCount
}

// IncrementScrapeCount increments the scrape call counter.
func (h *ToolHandlers) IncrementScrapeCount() {
	h.scrapeCount++
}

// HasReachedTarget returns true if we've saved enough products.
func (h *ToolHandlers) HasReachedTarget() bool {
	return len(h.savedProducts) >= h.minProducts
}

// productNameList returns a comma-separated list of saved product names.
func (h *ToolHandlers) productNameList() string {
	names := make([]string, len(h.savedProducts))
	for i, p := range h.savedProducts {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}
