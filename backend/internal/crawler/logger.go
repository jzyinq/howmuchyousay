package crawler

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CrawlLogger writes structured log entries to a per-crawl log file.
type CrawlLogger struct {
	file     *os.File
	filePath string
	mu       sync.Mutex
}

// NewCrawlLogger creates a new logger that writes to logs/crawl_<crawlID>.log.
// Creates the directory if it doesn't exist.
func NewCrawlLogger(logDir string, crawlID uuid.UUID) (*CrawlLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("creating log directory %s: %w", logDir, err)
	}

	filename := fmt.Sprintf("crawl_%s.log", crawlID.String())
	filePath := filepath.Join(logDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("creating log file %s: %w", filePath, err)
	}

	return &CrawlLogger{
		file:     file,
		filePath: filePath,
	}, nil
}

// Log writes a structured log entry with timestamp and level to both the log
// file and stdout.
// Level should be one of: FETCH, PARSE, AI_REQUEST, AI_RESPONSE, NAVIGATE,
// PRODUCT_FOUND, VALIDATION, ERROR.
func (l *CrawlLogger) Log(level string, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	entry := fmt.Sprintf("%s [%s] %s\n", timestamp, level, message)

	l.file.WriteString(entry)
	os.Stdout.WriteString(entry)
}

// FilePath returns the path to the log file.
func (l *CrawlLogger) FilePath() string {
	return l.filePath
}

// Close closes the log file.
func (l *CrawlLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
