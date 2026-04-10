package crawler

import (
	"fmt"
	"strings"
	"sync"
)

// ToolHandlers manages crawl state for the orchestrator loop: visited URLs,
// saved products, scrape count, and deduplication.
type ToolHandlers struct {
	mu            sync.Mutex
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
	h.mu.Lock()
	defer h.mu.Unlock()

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
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.savedProducts) == 0 {
		return fmt.Sprintf("Progress: 0/%d products. No products saved yet.", h.minProducts)
	}
	return fmt.Sprintf("Progress: %d/%d products.\nSaved so far: %s",
		len(h.savedProducts), h.minProducts, h.productNameList())
}

// SavedProducts returns a copy of all saved products.
func (h *ToolHandlers) SavedProducts() []RawProduct {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RawProduct, len(h.savedProducts))
	copy(out, h.savedProducts)
	return out
}

// MinProducts returns the target product count.
func (h *ToolHandlers) MinProducts() int {
	return h.minProducts
}

// IsVisited checks if a URL has already been scraped.
func (h *ToolHandlers) IsVisited(url string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.visitedURLs[url]
}

// MarkVisited records a URL as already scraped.
func (h *ToolHandlers) MarkVisited(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.visitedURLs[url] = true
}

// ScrapeCount returns the number of Firecrawl scrape calls made.
func (h *ToolHandlers) ScrapeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.scrapeCount
}

// TryIncrementScrapeCount atomically checks the safety cap and increments if allowed.
// Returns true if the increment was allowed, false if the cap was reached.
func (h *ToolHandlers) TryIncrementScrapeCount(maxScrapes int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.scrapeCount >= maxScrapes {
		return false
	}
	h.scrapeCount++
	return true
}

// HasReachedTarget returns true if we've saved enough products.
func (h *ToolHandlers) HasReachedTarget() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
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
