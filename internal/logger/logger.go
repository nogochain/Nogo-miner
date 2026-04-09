// Package logger provides structured logging with rotation support
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents log level
type Level int

const (
	// DebugLevel logs all messages
	DebugLevel Level = iota
	// InfoLevel logs informational messages
	InfoLevel
	// WarnLevel logs warning messages
	WarnLevel
	// ErrorLevel logs error messages
	ErrorLevel
)

// String returns level as string
func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses level string
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn":
		return WarnLevel
	case "error":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

// Config represents logger configuration
type Config struct {
	Level      string
	File       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
	JSONFormat bool
}

// Logger represents a structured logger
type Logger struct {
	mu         sync.Mutex
	level      Level
	file       string
	maxSize    int64
	maxBackups int
	maxAge     time.Duration
	compress   bool
	jsonFormat bool
	writer     io.Writer
	fileWriter *os.File
}

// Entry represents a log entry
type Entry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Caller    string                 `json:"caller,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// New creates a new logger
func New(cfg Config) (*Logger, error) {
	logger := &Logger{
		level:      ParseLevel(cfg.Level),
		file:       cfg.File,
		maxSize:    int64(cfg.MaxSizeMB) * 1024 * 1024,
		maxBackups: cfg.MaxBackups,
		maxAge:     time.Duration(cfg.MaxAgeDays) * 24 * time.Hour,
		compress:   cfg.Compress,
		jsonFormat: cfg.JSONFormat,
	}

	// Default to stdout
	logger.writer = os.Stdout

	// Try to open file
	if cfg.File != "" {
		if err := logger.openFile(); err != nil {
			// Log error but continue with stdout
			fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		}
	}

	return logger, nil
}

// openFile opens log file with rotation
func (l *Logger) openFile() error {
	// Check if file exists and get size
	fileInfo, err := os.Stat(l.file)
	if err == nil {
		// File exists, check if rotation needed
		if fileInfo.Size() > l.maxSize {
			if err := l.rotate(); err != nil {
				return fmt.Errorf("rotate log: %w", err)
			}
		}
	}

	// Open file for appending
	f, err := os.OpenFile(l.file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	l.fileWriter = f
	l.writer = f

	// Clean old backups
	go l.cleanOldBackups()

	return nil
}

// rotate rotates log file
func (l *Logger) rotate() error {
	if l.file == "" {
		return nil
	}

	// Generate backup filename
	timestamp := time.Now().Format("20060102-150405")
	dir := filepath.Dir(l.file)
	base := filepath.Base(l.file)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	backupPath := filepath.Join(dir, fmt.Sprintf("%s-%s%s%s", name, timestamp, "-backup", ext))

	// Close current file
	if l.fileWriter != nil {
		l.fileWriter.Close()
	}

	// Rename current file to backup
	if err := os.Rename(l.file, backupPath); err != nil {
		return fmt.Errorf("rename log file: %w", err)
	}

	// Compress if enabled
	if l.compress {
		go l.compressBackup(backupPath)
	}

	// Open new file
	return l.openFile()
}

// compressBackup compresses backup file (stub - would use gzip in production)
func (l *Logger) compressBackup(backupPath string) {
	// In production: use gzip to compress
	// For now, just log
	fmt.Printf("Would compress: %s\n", backupPath)
}

// cleanOldBackups removes old backup files
func (l *Logger) cleanOldBackups() {
	if l.maxAge <= 0 {
		return
	}

	dir := filepath.Dir(l.file)
	base := filepath.Base(l.file)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-l.maxAge)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// Check if it's a backup file
		if strings.Contains(filename, name) && strings.Contains(filename, "-backup") {
			filePath := filepath.Join(dir, filename)
			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.ModTime().Before(cutoff) {
				os.Remove(filePath)
			}
		}
	}
}

// log writes a log entry
func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Create entry
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level.String(),
		Message:   msg,
		Fields:    fields,
	}

	// Add caller info for debug level
	if level == DebugLevel {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			entry.Caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}

	// Write entry
	if l.jsonFormat {
		data, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to marshal log entry: %v\n", err)
			return
		}
		fmt.Fprintln(l.writer, string(data))
	} else {
		// Text format with ANSI colors
		color := getColor(level)
		reset := "\033[0m"
		timestamp := entry.Timestamp[11:19] // HH:MM:SS
		
		var output string
		if entry.Caller != "" {
			output = fmt.Sprintf("%s%s %s%-5s%s [%s] %s", 
				timestamp, color, color, entry.Level, reset, entry.Caller, entry.Message)
		} else {
			output = fmt.Sprintf("%s%s %s%-5s%s %s", 
				timestamp, color, color, entry.Level, reset, entry.Message)
		}
		
		if len(fields) > 0 {
			output += " " + formatFields(fields)
		}
		
		fmt.Fprintln(l.writer, output)
	}

	// Flush
	if f, ok := l.writer.(*os.File); ok {
		f.Sync()
	}
}

// getColor returns ANSI color code for level
func getColor(level Level) string {
	switch level {
	case DebugLevel:
		return "\033[36m" // Cyan
	case InfoLevel:
		return "\033[32m" // Green
	case WarnLevel:
		return "\033[33m" // Yellow
	case ErrorLevel:
		return "\033[31m" // Red
	default:
		return "\033[0m"
	}
}

// formatFields formats fields as string
func formatFields(fields map[string]interface{}) string {
	parts := make([]string, 0, len(fields))
	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}

// Debug logs debug message
func (l *Logger) Debug(msg string) {
	l.log(DebugLevel, msg, nil)
}

// Debugf logs debug message with format
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// DebugWithFields logs debug message with fields
func (l *Logger) DebugWithFields(msg string, fields map[string]interface{}) {
	l.log(DebugLevel, msg, fields)
}

// Info logs info message
func (l *Logger) Info(msg string) {
	l.log(InfoLevel, msg, nil)
}

// Infof logs info message with format
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// InfoWithFields logs info message with fields
func (l *Logger) InfoWithFields(msg string, fields map[string]interface{}) {
	l.log(InfoLevel, msg, fields)
}

// Warn logs warning message
func (l *Logger) Warn(msg string) {
	l.log(WarnLevel, msg, nil)
}

// Warnf logs warning message with format
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// WarnWithFields logs warning message with fields
func (l *Logger) WarnWithFields(msg string, fields map[string]interface{}) {
	l.log(WarnLevel, msg, fields)
}

// Error logs error message
func (l *Logger) Error(msg string) {
	l.log(ErrorLevel, msg, nil)
}

// Errorf logs error message with format
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// ErrorWithFields logs error message with fields
func (l *Logger) ErrorWithFields(msg string, fields map[string]interface{}) {
	l.log(ErrorLevel, msg, fields)
}

// SetLevel sets log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// GetLevel returns current log level
func (l *Logger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// Close closes logger
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fileWriter != nil {
		return l.fileWriter.Close()
	}
	return nil
}

// WithFields creates a new logger with fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	// In production: would use context or entry-based logging
	// For now, just return same logger
	return l
}
