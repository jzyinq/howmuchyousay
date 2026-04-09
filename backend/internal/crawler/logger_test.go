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

	logger.Log("FETCH", "Fetching https://example.com, status 200, 15000 bytes")
	logger.Log("PARSE", "Found 2 JSON-LD products")
	logger.Log("PRODUCT_FOUND", "Product: Laptop Dell, Price: 5999.99")
	logger.Log("ERROR", "Failed to parse page: invalid HTML")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)

	text := string(content)
	assert.Contains(t, text, "[FETCH]")
	assert.Contains(t, text, "https://example.com")
	assert.Contains(t, text, "[PARSE]")
	assert.Contains(t, text, "[PRODUCT_FOUND]")
	assert.Contains(t, text, "[ERROR]")

	// Each line should have a timestamp
	lines := strings.Split(strings.TrimSpace(text), "\n")
	assert.Len(t, lines, 4)
	for _, line := range lines {
		// Timestamp format: 2026-04-09T12:00:00Z
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

	logger.Log("FETCH", "test entry")
	logger.Close()

	content, err := os.ReadFile(logger.FilePath())
	require.NoError(t, err)
	assert.Contains(t, string(content), "[FETCH]")
}
