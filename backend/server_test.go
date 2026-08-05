package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunServerRejectsUnexpectedArguments(t *testing.T) {
	err := runServer([]string{"unexpected"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unexpected argument "unexpected"`) {
		t.Fatalf("error = %v", err)
	}
}
