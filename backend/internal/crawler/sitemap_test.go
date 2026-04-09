package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSampleURLs_ReturnsAllWhenFewerThanN(t *testing.T) {
	urls := []string{"http://a.com/1", "http://a.com/2", "http://a.com/3"}
	result := crawler.SampleURLs(urls, 10)
	assert.Len(t, result, 3)
	assert.ElementsMatch(t, urls, result)
}

func TestSampleURLs_ReturnsExactlyN(t *testing.T) {
	urls := make([]string, 200)
	for i := range urls {
		urls[i] = fmt.Sprintf("http://a.com/%d", i)
	}
	result := crawler.SampleURLs(urls, 100)
	assert.Len(t, result, 100)

	urlSet := make(map[string]bool)
	for _, u := range urls {
		urlSet[u] = true
	}
	for _, u := range result {
		assert.True(t, urlSet[u], "sampled URL should be from original list")
	}
}

func TestSampleURLs_EmptyInput(t *testing.T) {
	result := crawler.SampleURLs(nil, 100)
	assert.Empty(t, result)
}

func TestSampleURLs_ZeroN(t *testing.T) {
	urls := []string{"http://a.com/1"}
	result := crawler.SampleURLs(urls, 0)
	assert.Empty(t, result)
}

func TestFetchSitemapURLs_SimpleURLSet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>http://` + r.Host + `/products/1</loc></url>
  <url><loc>http://` + r.Host + `/products/2</loc></url>
  <url><loc>http://` + r.Host + `/products/3</loc></url>
</urlset>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	require.NoError(t, err)
	assert.Len(t, urls, 3)
	assert.Contains(t, urls, server.URL+"/products/1")
	assert.Contains(t, urls, server.URL+"/products/2")
	assert.Contains(t, urls, server.URL+"/products/3")
}

func TestFetchSitemapURLs_SitemapIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>http://` + r.Host + `/sitemap-products.xml</loc></sitemap>
</sitemapindex>`))
		case "/sitemap-products.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>http://` + r.Host + `/product/a</loc></url>
  <url><loc>http://` + r.Host + `/product/b</loc></url>
</urlset>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	require.NoError(t, err)
	assert.Len(t, urls, 2)
	assert.Contains(t, urls, server.URL+"/product/a")
	assert.Contains(t, urls, server.URL+"/product/b")
}

func TestFetchSitemapURLs_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	assert.NoError(t, err)
	assert.Nil(t, urls)
}

func TestFetchSitemapURLs_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Write([]byte(`<html><body>Not XML sitemap</body></html>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	assert.NoError(t, err)
	assert.Nil(t, urls)
}

func TestFetchSitemapURLs_EmptySitemap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sitemap.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	assert.NoError(t, err)
	assert.Nil(t, urls)
}

func TestFetchSitemapURLs_RobotsTxtFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.WriteHeader(http.StatusNotFound)
		case "/robots.txt":
			w.Write([]byte("User-agent: *\nAllow: /\nSitemap: http://" + r.Host + "/custom-sitemap.xml\n"))
		case "/custom-sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>http://` + r.Host + `/item/x</loc></url>
  <url><loc>http://` + r.Host + `/item/y</loc></url>
</urlset>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := crawler.NewHTTPFetcher(5 * time.Second)
	ctx := context.Background()

	urls, err := crawler.FetchSitemapURLs(ctx, fetcher, server.URL)
	require.NoError(t, err)
	assert.Len(t, urls, 2)
	assert.Contains(t, urls, server.URL+"/item/x")
	assert.Contains(t, urls, server.URL+"/item/y")
}
