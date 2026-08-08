package config

import (
	"fmt"
	"log/slog"
	"strings"

	"go.yaml.in/yaml/v3"
)

// LogLevel is a validated slog logging threshold.
type LogLevel slog.Level

func (l *LogLevel) UnmarshalYAML(node *yaml.Node) error {
	return l.UnmarshalText([]byte(node.Value))
}

// UnmarshalText parses a standard slog level name or offset.
func (l *LogLevel) UnmarshalText(text []byte) error {
	level, err := ParseLogLevel(string(text))
	if err != nil {
		return err
	}

	*l = LogLevel(level)

	return nil
}

// Level returns the configured threshold as a slog level.
func (l LogLevel) Level() slog.Level {
	return slog.Level(l)
}

// ParseLogLevel converts a configured slog level name or offset into a level.
func ParseLogLevel(value string) (slog.Level, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "WARNING" {
		normalized = "WARN"
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(normalized)); err != nil {
		return 0, fmt.Errorf("invalid server.log_level %q: %w", value, err)
	}

	return level, nil
}
