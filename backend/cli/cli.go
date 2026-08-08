package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/certvault/certvault/config"
)

// IsInvocation reports whether args request a command instead of the server.
func IsInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}

	switch args[0] {
	case "api-key", "check-config", "help", "-h", "--help":
		return true
	default:
		return false
	}
}

// Run dispatches the CertVault command line.
func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("a command is required")
	}

	var err error

	switch args[0] {
	case "api-key":
		err = RunAPIKey(args[1:], stdout, stderr)
	case "check-config":
		err = checkConfig(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	return err
}

func checkConfig(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("check-config", flag.ContinueOnError)
	flags.SetOutput(stderr)

	configPath := flags.String("config", configuredPath(), "configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf("check-config does not accept positional arguments")
	}

	if _, err := config.Load(*configPath); err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	_, err := fmt.Fprintln(stdout, "configuration valid")

	return err
}

func printUsage(output io.Writer) {
	_, _ = fmt.Fprintln(output, "usage: certvault <check-config|api-key> [options]")
}
