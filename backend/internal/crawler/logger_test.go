package crawler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrawlLogger_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	crawlID := uuid.New()

	logger, err := crawler.NewCrawlLogger(tmpDir, crawlID)
	require.NoError(t, err)
	defer logger.Close()

	expectedPath := filepath.Join(tmpDir, "crawl_"+crawlID.String()+".log")
	assert.Equal(t, expectedPath, logger.FilePath())

	_, err = os.Stat(expectedPath)
	assert.NoError(t, err, "log file should exist")
}

func TestCrawlLogger_WritesEntries(t *testing.T) {
	tmpDir := t.TempDir()
	crawlID := uuid.New()

	logger, err := crawler.NewCrawlLogger(tmpDir, crawlID)
	require.NoError(t, err)

	logger.Log("FIRECRAWL_START", "Starting Firecrawl job for https://example.com")
	logger.Log("FIRECRAWL_PROGRESS", "Crawled 5/10 pages")
	logger.Log("FIRECRAWL_COMPLETE", "Firecrawl job finished, 10 pages returned")
	logger.Log("AI_REQUEST", "Sending markdown to AI for extraction")
	logger.Log("PRODUCT_FOUND", "Product: Laptop Dell, Price: 5999.99")
	logger.Log("ERROR", "Failed to extract from page")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)

	text := string(content)
	assert.Contains(t, text, "[FIRECRAWL_START]")
	assert.Contains(t, text, "[FIRECRAWL_PROGRESS]")
	assert.Contains(t, text, "[FIRECRAWL_COMPLETE]")
	assert.Contains(t, text, "[AI_REQUEST]")
	assert.Contains(t, text, "[PRODUCT_FOUND]")
	assert.Contains(t, text, "[ERROR]")

	lines := strings.Split(strings.TrimSpace(text), "\n")
	assert.Len(t, lines, 6)
	for _, line := range lines {
		assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, line)
	}
}

func TestCrawlLogger_CreatesDirectoryIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "logs", "crawls")
	crawlID := uuid.New()

	logger, err := crawler.NewCrawlLogger(nestedDir, crawlID)
	require.NoError(t, err)
	defer logger.Close()

	logger.Log("FIRECRAWL_START", "test entry")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "[FIRECRAWL_START]")
}
