package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
)

var validScopes = map[string]bool{
	"certificates:read": true,
	"private_keys:read": true,
	"renewals:trigger":  true,
}

type stringList []string

func (values *stringList) String() string { return strings.Join(*values, ",") }

func (values *stringList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("value cannot be empty")
	}

	*values = append(*values, value)

	return nil
}

// RunAPIKey executes local API-key management against CertVault's configured database.
func RunAPIKey(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printAPIKeyUsage(stderr)
		return errors.New("a command is required")
	}

	var err error

	switch args[0] {
	case "create":
		err = createAPIKey(args[1:], stdout, stderr)
	case "list":
		err = listAPIKeys(args[1:], stdout, stderr)
	case "revoke":
		err = revokeAPIKey(args[1:], stdout, stderr)
	case "delete":
		err = deleteAPIKey(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printAPIKeyUsage(stdout)
		return nil
	default:
		printAPIKeyUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}

	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	return err
}

func createAPIKey(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("api-key create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", configuredPath(), "configuration file")
	name := flags.String("name", "", "API key name")
	expiresAtValue := flags.String("expires-at", "", "expiration time in RFC3339 format")

	var (
		scopes       stringList
		certificates stringList
	)

	flags.Var(&scopes, "scope", "scope to grant; may be repeated")
	flags.Var(&certificates, "certificate", "certificate name to allow; may be repeated or set to *")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return errors.New("create does not accept positional arguments")
	}

	if strings.TrimSpace(*name) == "" {
		return errors.New("--name is required")
	}

	if len(scopes) == 0 {
		return errors.New("at least one --scope is required")
	}

	for _, scope := range scopes {
		if !validScopes[scope] {
			return fmt.Errorf("unsupported scope %q", scope)
		}
	}

	if len(certificates) == 0 {
		return errors.New("at least one --certificate is required")
	}

	var expiresAt *time.Time

	if *expiresAtValue != "" {
		parsed, err := time.Parse(time.RFC3339, *expiresAtValue)
		if err != nil {
			return fmt.Errorf("parse --expires-at: %w", err)
		}

		expiresAt = &parsed
	}

	return withRepositories(*configPath, func(repositories *repository.Repositories) error {
		key, token, err := repositories.APIKeys.Create(
			context.Background(), strings.TrimSpace(*name), scopes, certificates, expiresAt,
		)
		if err != nil {
			return err
		}

		repositories.Audits.Record(
			context.Background(), audit.ActorLocalCLI,
			audit.ActionAPIKeyCreate, key.Name, "", "",
		)

		_, err = fmt.Fprintln(stdout, token)

		return err
	})
}

func listAPIKeys(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("api-key list", flag.ContinueOnError)
	flags.SetOutput(stderr)

	configPath := flags.String("config", configuredPath(), "configuration file")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return errors.New("list does not accept positional arguments")
	}

	return withRepositories(*configPath, func(repositories *repository.Repositories) error {
		keys, err := repositories.APIKeys.List(context.Background())
		if err != nil {
			return err
		}

		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")

		return encoder.Encode(keys)
	})
}

func revokeAPIKey(args []string, stdout, stderr io.Writer) error {
	return mutateAPIKey("revoke", "revoked", audit.ActionAPIKeyRevoke, args, stdout, stderr, func(
		ctx context.Context, repositories *repository.Repositories, id int64,
	) (string, error) {
		return repositories.APIKeys.Revoke(ctx, id)
	})
}

func deleteAPIKey(args []string, stdout, stderr io.Writer) error {
	return mutateAPIKey("delete", "deleted", audit.ActionAPIKeyDelete, args, stdout, stderr, func(
		ctx context.Context, repositories *repository.Repositories, id int64,
	) (string, error) {
		return repositories.APIKeys.Delete(ctx, id)
	})
}

func mutateAPIKey(
	action string,
	completedAction string,
	auditAction audit.Action,
	args []string,
	stdout, stderr io.Writer,
	mutation func(context.Context, *repository.Repositories, int64) (string, error),
) error {
	flags := flag.NewFlagSet("api-key "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", configuredPath(), "configuration file")

	idValue := flags.String("id", "", "API key ID")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 0 {
		return fmt.Errorf("%s does not accept positional arguments", action)
	}

	id, err := strconv.ParseInt(*idValue, 10, 64)
	if err != nil || id <= 0 {
		return errors.New("--id must be a positive integer")
	}

	return withRepositories(*configPath, func(repositories *repository.Repositories) error {
		ctx := context.Background()

		name, err := mutation(ctx, repositories, id)
		if err != nil {
			return err
		}

		repositories.Audits.Record(
			ctx, audit.ActorLocalCLI, auditAction, name, "", "",
		)
		_, err = fmt.Fprintf(stdout, "%s API key %d (%s)\n", completedAction, id, name)

		return err
	})
}

func withRepositories(configPath string, run func(*repository.Repositories) error) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	db, err := database.Open(filepath.Join(cfg.DataDir, "certvault.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	defer func() { _ = db.Close() }()

	repositories := repository.New(db)
	if err := repositories.Certificates.Reconcile(context.Background(), cfg); err != nil {
		return fmt.Errorf("reconcile configuration: %w", err)
	}

	return run(repositories)
}

func configuredPath() string {
	return config.Path()
}

func printAPIKeyUsage(output io.Writer) {
	_, _ = fmt.Fprintln(output, "usage: certvault api-key <create|list|revoke|delete> [options]")
}
