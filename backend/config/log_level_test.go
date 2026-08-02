package config

import (
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug":     slog.LevelDebug,
		"INFO":      slog.LevelInfo,
		" warning ": slog.LevelWarn,
		"error":     slog.LevelError,
		"DEBUG+2":   slog.LevelDebug + 2,
	}

	for input, expected := range tests {
		actual, err := ParseLogLevel(input)
		if err != nil {
			t.Errorf("ParseLogLevel(%q): %v", input, err)
			continue
		}
		if actual != expected {
			t.Errorf("ParseLogLevel(%q) = %v, want %v", input, actual, expected)
		}
	}
}

func TestParseLogLevelRejectsInvalidValue(t *testing.T) {
	if _, err := ParseLogLevel("verbose"); err == nil {
		t.Fatal("ParseLogLevel accepted an invalid value")
	}
}
