package jsonlog

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLoggerPrintInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, LevelInfo)
	logger.PrintInfo("hello", map[string]string{"k": "v"})

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output not JSON: %v", err)
	}
	if entry["level"] != "INFO" || entry["message"] != "hello" {
		t.Fatalf("unexpected entry: %v", entry)
	}
}

func TestLoggerBelowMinLevelSuppressed(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, LevelError)
	logger.PrintInfo("skip me", nil)
	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}
