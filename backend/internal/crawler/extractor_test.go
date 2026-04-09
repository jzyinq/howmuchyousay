package crawler_test

import (
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractProducts_JSONLD_SingleProduct(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "Product",
		"name": "iPhone 15 Pro",
		"image": "https://example.com/iphone.jpg",
		"offers": {
			"@type": "Offer",
			"price": "5499.00",
			"priceCurrency": "PLN"
		}
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/iphone")
	require.Len(t, products, 1)
	assert.Equal(t, "iPhone 15 Pro", products[0].Name)
	assert.Equal(t, 5499.00, products[0].Price)
	assert.Equal(t, "https://example.com/iphone.jpg", products[0].ImageURL)
	assert.Equal(t, "https://example.com/iphone", products[0].SourceURL)
}

func TestExtractProducts_JSONLD_MultipleProducts(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	[
		{
			"@context": "https://schema.org",
			"@type": "Product",
			"name": "Product A",
			"offers": {"price": "100.00"}
		},
		{
			"@context": "https://schema.org",
			"@type": "Product",
			"name": "Product B",
			"offers": {"price": "200.00"}
		}
	]
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/list")
	require.Len(t, products, 2)
	assert.Equal(t, "Product A", products[0].Name)
	assert.Equal(t, "Product B", products[1].Name)
}

func TestExtractProducts_JSONLD_OffersArray(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@type": "Product",
		"name": "Multi-offer Product",
		"offers": [
			{"price": "99.99"},
			{"price": "109.99"}
		]
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/multi")
	require.Len(t, products, 1)
	assert.Equal(t, "Multi-offer Product", products[0].Name)
	// Takes first offer price
	assert.Equal(t, 99.99, products[0].Price)
}

func TestExtractProducts_JSONLD_ImageArray(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@type": "Product",
		"name": "Image Array Product",
		"image": ["https://example.com/img1.jpg", "https://example.com/img2.jpg"],
		"offers": {"price": "50.00"}
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/images")
	require.Len(t, products, 1)
	assert.Equal(t, "https://example.com/img1.jpg", products[0].ImageURL)
}

func TestExtractProducts_JSONLD_NestedGraph(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@graph": [
			{
				"@type": "WebPage",
				"name": "Some page"
			},
			{
				"@type": "Product",
				"name": "Nested Product",
				"offers": {"price": "299.99"}
			}
		]
	}
	</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/nested")
	require.Len(t, products, 1)
	assert.Equal(t, "Nested Product", products[0].Name)
	assert.Equal(t, 299.99, products[0].Price)
}

func TestExtractProducts_OpenGraph(t *testing.T) {
	html := `<html><head>
	<meta property="og:type" content="product" />
	<meta property="og:title" content="OG Product" />
	<meta property="product:price:amount" content="199.99" />
	<meta property="og:image" content="https://example.com/og.jpg" />
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/og")
	require.Len(t, products, 1)
	assert.Equal(t, "OG Product", products[0].Name)
	assert.Equal(t, 199.99, products[0].Price)
	assert.Equal(t, "https://example.com/og.jpg", products[0].ImageURL)
}

func TestExtractProducts_NoStructuredData(t *testing.T) {
	html := `<html><head><title>Just a page</title></head>
	<body><h1>No products here</h1></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/empty")
	assert.Empty(t, products)
}

func TestExtractProducts_InvalidJSON(t *testing.T) {
	html := `<html><head>
	<script type="application/ld+json">{invalid json}</script>
	</head><body></body></html>`

	products := crawler.ExtractProducts(html, "https://example.com/broken")
	assert.Empty(t, products)
}

func TestExtractLinks(t *testing.T) {
	html := `<html><body>
	<a href="/products/1">Product 1</a>
	<a href="/products/2">Product 2</a>
	<a href="/category/electronics">Electronics</a>
	<a href="https://external.com/page">External</a>
	<a href="#">Empty</a>
	</body></html>`

	links := crawler.ExtractLinks(html, "https://example.com")
	assert.Contains(t, links, "https://example.com/products/1")
	assert.Contains(t, links, "https://example.com/products/2")
	assert.Contains(t, links, "https://example.com/category/electronics")
	// External links included
	assert.Contains(t, links, "https://external.com/page")
	// Fragment-only links excluded
	assert.NotContains(t, links, "#")
}
