package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
	"golang.org/x/term"
)

func usage() {
	out := flag.CommandLine.Output()
	_, _ = fmt.Fprintf(out, "Usage:\n  vimmary [flags]                                   serve\n")
	_, _ = fmt.Fprintf(out, "  vimmary [flags] auth create-user [--admin] <username>\n")
	_, _ = fmt.Fprintf(out, "      create a password user; the password is read from stdin\n")
	_, _ = fmt.Fprintf(out, "  vimmary [flags] auth create-token --user <login> --name <name> --scope mcp|api [--scope …] [--expires <duration>]\n")
	_, _ = fmt.Fprintf(out, "      create a personal access token and print it to stdout\n\nFlags:\n")
	flag.PrintDefaults()
}

// runCommand runs a subcommand given after the global flags.
func runCommand(configPath string, args []string, log *slog.Logger) error {
	if len(args) >= 2 && args[0] == "auth" {
		switch args[1] {
		case "create-user":
			return createUserCommand(configPath, args[2:], log)
		case "create-token":
			return createTokenCommand(configPath, args[2:], os.Stdout, log)
		}
	}
	usage()
	return fmt.Errorf("unknown command %q", strings.Join(args, " "))
}

// scopeFlags collects --scope, repeated or comma-separated.
type scopeFlags []string

func (s *scopeFlags) String() string { return strings.Join(*s, ",") }

func (s *scopeFlags) Set(v string) error {
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			*s = append(*s, p)
		}
	}
	return nil
}

// parseExpiry reads a Go duration, or a whole number of days as "<n>d".
// Empty means no expiry.
func parseExpiry(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	if n, ok := strings.CutSuffix(v, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("--expires %q: want a positive number of days or a Go duration", v)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("--expires %q: want a positive number of days or a Go duration", v)
	}
	return d, nil
}

// createTokenCommand creates a personal access token for an existing user and
// writes the secret, and nothing else, to out. It is how machine clients get
// their tokens on an instance without a browser at hand.
func createTokenCommand(configPath string, args []string, out io.Writer, log *slog.Logger) error {
	fs := flag.NewFlagSet("auth create-token", flag.ContinueOnError)
	user := fs.String("user", "", "login of the user the token acts as")
	name := fs.String("name", "", "name of the token, shown in Settings")
	expires := fs.String("expires", "", "lifetime, e.g. 90d or 720h; empty: no expiry")
	var scopes scopeFlags
	fs.Var(&scopes, "scope", "mcp or api; repeat or comma-separate for both")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *user == "" || *name == "" || len(scopes) == 0 {
		return errors.New("usage: vimmary auth create-token --user <login> --name <name> --scope mcp|api [--expires <duration>]")
	}
	lifetime, err := parseExpiry(*expires)
	if err != nil {
		return err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Auth == nil {
		return fmt.Errorf("%s has no auth block; tokens need application login", configPath)
	}

	database, _, err := openDatabase(cfg, false, false, log)
	if err != nil {
		return err
	}
	defer database.Close()
	store := storage.NewDB(database)
	accounts := service.NewAccounts(store, log)
	if _, err := newAuth(*cfg.Auth, store, accounts, log); err != nil {
		return err
	}

	var expiresAt time.Time
	if lifetime > 0 {
		expiresAt = time.Now().Add(lifetime)
	}
	tok, err := accounts.CreateTokenForLogin(context.Background(), *user, *name, scopes, expiresAt)
	if err != nil {
		return errors.New(strings.TrimPrefix(err.Error(), service.ErrAccountInvalid.Error()+": "))
	}
	log.Info("token created", "id", tok.ID, "user_id", tok.UserID, "name", tok.Name, "scopes", tok.Scopes, "expires_at", tok.ExpiresAt)
	_, err = fmt.Fprintln(out, tok.Secret)
	return err
}

// createUserCommand creates a password user, or gives an existing user
// without a password a password login. It is how the first admin of a
// password-only instance comes into being. The password never appears as an
// argument: it is prompted for on a terminal and read from stdin otherwise.
func createUserCommand(configPath string, args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("auth create-user", flag.ContinueOnError)
	admin := fs.Bool("admin", false, "make the user an admin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: vimmary auth create-user [--admin] <username>")
	}
	username := strings.TrimSpace(fs.Arg(0))

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Auth == nil || !cfg.Auth.HasMethod(auth.MethodPassword) {
		return fmt.Errorf("auth.methods in %s must include %q", configPath, auth.MethodPassword)
	}

	password, err := readPassword(os.Stdin, os.Stderr)
	if err != nil {
		return err
	}

	database, _, err := openDatabase(cfg, false, false, log)
	if err != nil {
		return err
	}
	defer database.Close()
	store := storage.NewDB(database)
	accounts := service.NewAccounts(store, log)
	if _, err := newAuth(*cfg.Auth, store, accounts, log); err != nil {
		return err
	}

	uid, created, err := accounts.EnsurePasswordUser(context.Background(), username, password, *admin)
	if err != nil {
		return errors.New(strings.TrimPrefix(err.Error(), service.ErrAccountInvalid.Error()+": "))
	}
	verb := "added a password login to existing user"
	if created {
		verb = "created user"
	}
	role := ""
	if *admin {
		role = " (admin)"
	}
	fmt.Printf("%s %q, id %d%s\n", verb, username, uid, role)
	return nil
}

// readPassword prompts twice on a terminal, or reads the first line of a
// non-terminal stdin.
func readPassword(in *os.File, prompt io.Writer) (string, error) {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read password: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return "", errors.New("no password on stdin")
		}
		return line, nil
	}
	_, _ = fmt.Fprint(prompt, "Password: ")
	first, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(prompt)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	_, _ = fmt.Fprint(prompt, "Repeat password: ")
	second, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(prompt)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if string(first) != string(second) {
		return "", errors.New("the passwords do not match")
	}
	return string(first), nil
}
