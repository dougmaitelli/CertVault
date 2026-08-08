package config

import (
	"encoding/base64"
	"errors"
	"strings"
)

type MasterKey []byte

func (key *MasterKey) UnmarshalText(text []byte) error {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(text)))
	if err != nil || len(decoded) != 32 {
		return errors.New("master key must be base64-encoded 32 bytes")
	}

	*key = decoded

	return nil
}
