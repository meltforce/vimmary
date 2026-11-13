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
	"strings"

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
	_, _ = fmt.Fprintf(out, "      create a password user; the password is read from stdin\n\nFlags:\n")
	flag.PrintDefaults()
}

// runCommand runs a subcommand given after the global flags.
func runCommand(configPath string, args []string, log *slog.Logger) error {
	if len(args) >= 2 && args[0] == "auth" && args[1] == "create-user" {
		return createUserCommand(configPath, args[2:], log)
	}
	usage()
	return fmt.Errorf("unknown command %q", strings.Join(args, " "))
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
