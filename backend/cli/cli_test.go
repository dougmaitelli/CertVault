package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"api-key", "--help"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "certvault api-key") {
		t.Fatalf("API-key help = %q", output.String())
	}

	output.Reset()

	if err := Run([]string{"--help"}, &output, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), "certvault <check-config|api-key>") {
		t.Fatalf("root help = %q", output.String())
	}
}

func TestRunCheckConfig(t *testing.T) {
	configPath, _ := writeTestConfig(t)

	var output bytes.Buffer
	if err := Run(
		[]string{"check-config", "--config", configPath},
		&output,
		&bytes.Buffer{},
	); err != nil {
		t.Fatal(err)
	}

	if output.String() != "configuration valid\n" {
		t.Fatalf("check-config output = %q", output.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := Run([]string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unknown command "unknown"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestIsInvocation(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: nil, want: false},
		{args: []string{"-config", "/tmp/config.yaml"}, want: false},
		{args: []string{"api-key", "list"}, want: true},
		{args: []string{"check-config", "--config", "/tmp/config.yaml"}, want: true},
		{args: []string{"--help"}, want: true},
	}
	for _, test := range tests {
		if got := IsInvocation(test.args); got != test.want {
			t.Fatalf("IsInvocation(%q) = %v, want %v", test.args, got, test.want)
		}
	}
}
