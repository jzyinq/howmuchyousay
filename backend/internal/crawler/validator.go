package crawler

import (
	"fmt"
	"net/url"
	"strings"
)

// RejectedProduct records why a product was rejected during validation.
type RejectedProduct struct {
	Product RawProduct
	Reason  string
}

// ValidateProducts validates and deduplicates a slice of raw products.
// Returns valid products and rejected products with reasons.
func ValidateProducts(raw []RawProduct) (valid []RawProduct, rejected []RejectedProduct) {
	seen := make(map[string]bool)

	for _, p := range raw {
		// Validate name
		if strings.TrimSpace(p.Name) == "" {
			rejected = append(rejected, RejectedProduct{
				Product: p,
				Reason:  "empty name",
			})
			continue
		}

		// Validate price
		if p.Price <= 0 {
			rejected = append(rejected, RejectedProduct{
				Product: p,
				Reason:  fmt.Sprintf("invalid price: %.2f (must be > 0)", p.Price),
			})
			continue
		}

		// Validate and fix image URL
		if p.ImageURL != "" {
			if _, err := url.ParseRequestURI(p.ImageURL); err != nil {
				// Invalid URL -> replace with empty (placeholder fallback)
				p.ImageURL = ""
			} else if !strings.HasPrefix(p.ImageURL, "http://") && !strings.HasPrefix(p.ImageURL, "https://") {
				p.ImageURL = ""
			}
		}

		// Deduplicate by normalized name
		normalized := NormalizeName(p.Name)
		if seen[normalized] {
			rejected = append(rejected, RejectedProduct{
				Product: p,
				Reason:  fmt.Sprintf("duplicate name (normalized: %q)", normalized),
			})
			continue
		}
		seen[normalized] = true

		valid = append(valid, p)
	}

	return valid, rejected
}

// NormalizeName normalizes a product name for deduplication:
// lowercase, collapse whitespace, trim.
func NormalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ToLower(name)

	// Collapse multiple spaces into one
	parts := strings.Fields(name)
	return strings.Join(parts, " ")
}
