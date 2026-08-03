package config

import (
	"time"

	"go.yaml.in/yaml/v3"
)

const DefaultRenewBefore = 30 * 24 * time.Hour

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	duration, err := time.ParseDuration(node.Value)
	d.Duration = duration
	return err
}
