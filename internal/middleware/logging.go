package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

// Logger represents a logger that writes to an io.Writer
type Logger struct {
	writer io.Writer
	level  string
}

// NewLogger creates a new logger
func NewLogger(writer io.Writer, level string) *Logger {
	return &Logger{
		writer: writer,
		level:  level,
	}
}

// NewDefaultLogger creates a logger that writes to stdout
func NewDefaultLogger(level string) *Logger {
	return NewLogger(os.Stdout, level)
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Level     string `json:"level"`
	Time      string `json:"time"`
	RequestID string `json:"request_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	Duration  int64  `json:"duration_ms"`
	Error     string `json:"error,omitempty"`
}

// Log writes a log entry
func (l *Logger) Log(entry *LogEntry) error {
	if l.writer == nil {
		return nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	_, err = l.writer.Write(append(data, '\n'))
	return err
}

// Logging returns a logging middleware
func Logging(writer io.Writer) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	logger := NewLogger(writer, "info")

	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			start := time.Now()
			requestID := uuid.New().String()

			// Set request ID in response header
			ctx.Response.Header.Set("X-Request-ID", requestID)

			// Call next handler
			next(ctx)

			// Calculate duration
			duration := time.Since(start).Milliseconds()

			// Create log entry
			entry := &LogEntry{
				Level:     logger.level,
				Time:      start.UTC().Format(time.RFC3339),
				RequestID: requestID,
				Method:    string(ctx.Method()),
				Path:      string(ctx.Path()),
				Status:    ctx.Response.StatusCode(),
				Duration:  duration,
			}

			// Log errors
			if ctx.Response.StatusCode() >= 400 {
				entry.Error = string(ctx.Response.Body())
			}

			_ = logger.Log(entry)
		}
	}
}
