package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const DefaultRenewBefore = 30 * 24 * time.Hour

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	value := strings.TrimSpace(node.Value)
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseInt(strings.TrimSuffix(value, "d"), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid day duration %q: %w", value, err)
		}
		const day = 24 * time.Hour
		if days > int64(^uint64(0)>>1)/int64(day) || days < -int64(^uint64(0)>>1)/int64(day) {
			return fmt.Errorf("day duration %q is out of range", value)
		}
		d.Duration = time.Duration(days) * day
		return nil
	}
	duration, err := time.ParseDuration(value)
	d.Duration = duration
	return err
}
