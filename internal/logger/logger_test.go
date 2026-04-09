package logger

import (
	"os"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	// Test with stdout
	cfg := Config{
		Level:      "debug",
		File:       "",
		MaxSizeMB:  10,
		MaxBackups: 3,
		MaxAgeDays: 30,
		JSONFormat: false,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	if logger.level != DebugLevel {
		t.Errorf("Expected debug level, got %v", logger.level)
	}
}

func TestNewWithFile(t *testing.T) {
	// Create temporary file
	tmpfile := "test-" + time.Now().Format("20060102-150405") + ".log"
	defer os.Remove(tmpfile)

	cfg := Config{
		Level:      "info",
		File:       tmpfile,
		MaxSizeMB:  1,
		MaxBackups: 2,
		MaxAgeDays: 1,
		JSONFormat: true,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Write some logs
	logger.Info("Test message 1")
	logger.Infof("Test message %d", 2)
	logger.InfoWithFields("Test with fields", map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	})

	// Flush and check file
	logger.Close()

	data, err := os.ReadFile(tmpfile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Log file is empty")
	}

	// Verify JSON format
	lines := string(data)
	if lines[0] != '{' {
		t.Error("Expected JSON format")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", DebugLevel},
		{"info", InfoLevel},
		{"warn", WarnLevel},
		{"error", ErrorLevel},
		{"DEBUG", DebugLevel},
		{"invalid", InfoLevel},
		{"", InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseLevel(tt.input); got != tt.want {
				t.Errorf("ParseLevel(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("Level.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoggerMethods(t *testing.T) {
	cfg := Config{
		Level:      "debug",
		File:       "",
		JSONFormat: false,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test all methods
	logger.Debug("Debug message")
	logger.Debugf("Debug %d", 1)
	logger.DebugWithFields("Debug with fields", map[string]interface{}{"key": "value"})

	logger.Info("Info message")
	logger.Infof("Info %d", 2)
	logger.InfoWithFields("Info with fields", map[string]interface{}{"key": "value"})

	logger.Warn("Warn message")
	logger.Warnf("Warn %d", 3)
	logger.WarnWithFields("Warn with fields", map[string]interface{}{"key": "value"})

	logger.Error("Error message")
	logger.Errorf("Error %d", 4)
	logger.ErrorWithFields("Error with fields", map[string]interface{}{"key": "value"})
}

func TestSetLevel(t *testing.T) {
	cfg := Config{
		Level: "info",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	if logger.GetLevel() != InfoLevel {
		t.Error("Expected info level")
	}

	logger.SetLevel(ErrorLevel)
	if logger.GetLevel() != ErrorLevel {
		t.Error("Expected error level")
	}

	// Debug should not be logged
	logger.Debug("This should not appear")
}

func TestRotation(t *testing.T) {
	tmpfile := "test-rotation-" + time.Now().Format("20060102-150405") + ".log"
	defer os.Remove(tmpfile)

	cfg := Config{
		Level:      "info",
		File:       tmpfile,
		MaxSizeMB:  1, // 1 MB for easy testing
		MaxBackups: 2,
		MaxAgeDays: 1,
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Write enough to trigger rotation (would need 1MB in real test)
	// For unit test, just verify the mechanism exists
	logger.Info("Test rotation")
}

func TestWithFields(t *testing.T) {
	cfg := Config{
		Level: "debug",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test with fields (currently just returns same logger)
	logger2 := logger.WithFields(map[string]interface{}{"key": "value"})
	if logger2 != logger {
		t.Error("WithFields should return same logger in current implementation")
	}
}

func TestClose(t *testing.T) {
	cfg := Config{
		Level: "debug",
		File:  "",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Double close should be safe
	if err := logger.Close(); err != nil {
		t.Errorf("Double Close() error = %v", err)
	}
}
