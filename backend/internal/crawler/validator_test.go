package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProducts_Valid(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Laptop Dell XPS", Price: 5999.99, ImageURL: "https://img.com/dell.jpg", SourceURL: "https://shop.com/dell"},
		{Name: "iPhone 15", Price: 4999.00, ImageURL: "https://img.com/iphone.jpg", SourceURL: "https://shop.com/iphone"},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Len(t, valid, 2)
	assert.Empty(t, rejected)
}

func TestValidateProducts_EmptyName(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "", Price: 100.00, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Empty(t, valid)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Reason, "empty name")
}

func TestValidateProducts_ZeroPrice(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Free Product", Price: 0, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Empty(t, valid)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Reason, "price")
}

func TestValidateProducts_NegativePrice(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Negative Price", Price: -10.00, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Empty(t, valid)
	require.Len(t, rejected, 1)
	assert.Contains(t, rejected[0].Reason, "price")
}

func TestValidateProducts_InvalidImageURL(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "No Image", Price: 100.00, ImageURL: "not-a-url", SourceURL: "https://shop.com/item"},
	}

	// Invalid image URL gets replaced with empty string (placeholder fallback),
	// product is still valid.
	valid, rejected := crawler.ValidateProducts(raw)
	require.Len(t, valid, 1)
	assert.Empty(t, rejected)
	assert.Equal(t, "", valid[0].ImageURL)
}

func TestValidateProducts_Deduplication(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "iPhone 15 Pro", Price: 5499.00, ImageURL: "", SourceURL: "https://shop.com/a"},
		{Name: "iphone 15 pro", Price: 5599.00, ImageURL: "", SourceURL: "https://shop.com/b"},
		{Name: "  iPhone  15  Pro  ", Price: 5299.00, ImageURL: "", SourceURL: "https://shop.com/c"},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Len(t, valid, 1)
	// First occurrence is kept
	assert.Equal(t, "iPhone 15 Pro", valid[0].Name)
	assert.Equal(t, 5499.00, valid[0].Price)
	require.Len(t, rejected, 2)
	assert.Contains(t, rejected[0].Reason, "duplicate")
}

func TestValidateProducts_MixedValidInvalid(t *testing.T) {
	raw := []crawler.RawProduct{
		{Name: "Valid Product", Price: 100.00, ImageURL: "https://img.com/valid.jpg", SourceURL: ""},
		{Name: "", Price: 50.00, ImageURL: "", SourceURL: ""},
		{Name: "Zero Price", Price: 0, ImageURL: "", SourceURL: ""},
		{Name: "Another Valid", Price: 200.00, ImageURL: "", SourceURL: ""},
	}

	valid, rejected := crawler.ValidateProducts(raw)
	assert.Len(t, valid, 2)
	assert.Len(t, rejected, 2)
}

func TestNormalizeName(t *testing.T) {
	assert.Equal(t, "iphone 15 pro", crawler.NormalizeName("  iPhone  15  Pro  "))
	assert.Equal(t, "laptop dell xps", crawler.NormalizeName("Laptop Dell XPS"))
	assert.Equal(t, "test", crawler.NormalizeName("  TEST  "))
}
