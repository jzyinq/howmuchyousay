package crawler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolHandlers_SaveProduct(t *testing.T) {
	t.Run("saves valid product and returns progress", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.SaveProduct(RawProduct{
			Name:      "iPhone 15 Pro",
			Price:     999.99,
			ImageURL:  "https://img.com/iphone.jpg",
			SourceURL: "https://shop.com/product/iphone",
		})

		assert.Contains(t, result, "Saved")
		assert.Contains(t, result, "iPhone 15 Pro")
		assert.Contains(t, result, "1/20")
		assert.Len(t, h.SavedProducts(), 1)
	})

	t.Run("rejects product with empty name", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.SaveProduct(RawProduct{
			Name:      "",
			Price:     100.00,
			SourceURL: "https://shop.com/product/1",
		})

		assert.Contains(t, result, "rejected")
		assert.Len(t, h.SavedProducts(), 0)
	})

	t.Run("rejects product with zero price", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.SaveProduct(RawProduct{
			Name:      "Free Thing",
			Price:     0,
			SourceURL: "https://shop.com/product/1",
		})

		assert.Contains(t, result, "rejected")
		assert.Len(t, h.SavedProducts(), 0)
	})

	t.Run("rejects duplicate product by name", func(t *testing.T) {
		h := NewToolHandlers(20)

		h.SaveProduct(RawProduct{Name: "iPhone 15 Pro", Price: 999.99, SourceURL: "https://shop.com/a"})
		result := h.SaveProduct(RawProduct{Name: "iphone 15 pro", Price: 899.99, SourceURL: "https://shop.com/b"})

		assert.Contains(t, result, "Duplicate")
		assert.Len(t, h.SavedProducts(), 1)
	})

	t.Run("rejects duplicate product by source URL", func(t *testing.T) {
		h := NewToolHandlers(20)

		h.SaveProduct(RawProduct{Name: "Product A", Price: 100, SourceURL: "https://shop.com/p/1"})
		result := h.SaveProduct(RawProduct{Name: "Product B", Price: 200, SourceURL: "https://shop.com/p/1"})

		assert.Contains(t, result, "Duplicate")
		assert.Len(t, h.SavedProducts(), 1)
	})
}

func TestToolHandlers_GetStatus(t *testing.T) {
	t.Run("returns status with no products", func(t *testing.T) {
		h := NewToolHandlers(20)

		result := h.GetStatus()

		assert.Contains(t, result, "0/20")
	})

	t.Run("returns status with saved products", func(t *testing.T) {
		h := NewToolHandlers(20)
		h.SaveProduct(RawProduct{Name: "iPhone", Price: 999, SourceURL: "https://shop.com/1"})
		h.SaveProduct(RawProduct{Name: "Laptop", Price: 1499, SourceURL: "https://shop.com/2"})

		result := h.GetStatus()

		assert.Contains(t, result, "2/20")
		assert.Contains(t, result, "iPhone")
		assert.Contains(t, result, "Laptop")
	})
}

func TestToolHandlers_VisitedURL(t *testing.T) {
	t.Run("tracks visited URLs", func(t *testing.T) {
		h := NewToolHandlers(20)

		assert.False(t, h.IsVisited("https://shop.com/a"))
		h.MarkVisited("https://shop.com/a")
		assert.True(t, h.IsVisited("https://shop.com/a"))
		assert.False(t, h.IsVisited("https://shop.com/b"))
	})
}

func TestToolHandlers_ScrapeCount(t *testing.T) {
	t.Run("tracks scrape count", func(t *testing.T) {
		h := NewToolHandlers(20)

		assert.Equal(t, 0, h.ScrapeCount())
		h.IncrementScrapeCount()
		h.IncrementScrapeCount()
		assert.Equal(t, 2, h.ScrapeCount())
	})
}

func TestToolHandlers_HasReachedTarget(t *testing.T) {
	t.Run("returns false before target", func(t *testing.T) {
		h := NewToolHandlers(2)
		h.SaveProduct(RawProduct{Name: "A", Price: 10, SourceURL: "https://shop.com/1"})
		assert.False(t, h.HasReachedTarget())
	})

	t.Run("returns true at target", func(t *testing.T) {
		h := NewToolHandlers(2)
		h.SaveProduct(RawProduct{Name: "A", Price: 10, SourceURL: "https://shop.com/1"})
		h.SaveProduct(RawProduct{Name: "B", Price: 20, SourceURL: "https://shop.com/2"})
		assert.True(t, h.HasReachedTarget())
	})
}
