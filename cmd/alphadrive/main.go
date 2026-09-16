package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/AlphaTechiess/alphadrive/internal/app"
	"github.com/AlphaTechiess/alphadrive/internal/config"
)

// Injected at build time via -ldflags "-X main.Version=... -X main.BuildDate=... -X main.Commit=..."
var (
	Version   = "dev"
	BuildDate = "unknown"
	Commit    = "none"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	case "create-user":
		createUser(os.Args[2:])
	case "reset-password":
		resetPassword(os.Args[2:])
	case "backup":
		runBackup(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "version", "-v", "--version":
		printVersion()
	default:
		usage()
		os.Exit(2)
	}
}

func printVersion() {
	fmt.Printf("AlphaDrive v%s (built: %s, commit: %s, %s/%s)\n", Version, BuildDate, Commit, runtime.GOOS, runtime.GOARCH)
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to JSON configuration")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		fatal(err)
	}
	defer a.Close()
	fatal(a.Serve(context.Background()))
}

func createUser(args []string) {
	fs := flag.NewFlagSet("create-user", flag.ExitOnError)
	configPath := fs.String("config", "", "path to JSON configuration")
	name := fs.String("name", "", "full name (optional)")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password (prefer ALPHADRIVE_PASSWORD)")
	admin := fs.Bool("admin", true, "create an administrator")
	_ = fs.Parse(args)
	if *username == "" {
		fatal(fmt.Errorf("--username is required"))
	}
	if *password == "" {
		*password = os.Getenv("ALPHADRIVE_PASSWORD")
	}
	if *password == "" {
		fatal(fmt.Errorf("provide --password or ALPHADRIVE_PASSWORD"))
	}
	if len(*password) < 12 {
		fatal(fmt.Errorf("password must be at least 12 characters"))
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		fatal(err)
	}
	defer a.Close()
	if err := a.CreateUserWithName(context.Background(), *name, *username, *password, *admin); err != nil {
		fatal(err)
	}
	slog.Info("user created successfully", "username", *username, "admin", *admin)
}

func resetPassword(args []string) {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	configPath := fs.String("config", "", "path to JSON configuration")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "new password (prefer ALPHADRIVE_PASSWORD)")
	_ = fs.Parse(args)
	if *username == "" {
		fatal(fmt.Errorf("--username is required"))
	}
	if *password == "" {
		*password = os.Getenv("ALPHADRIVE_PASSWORD")
	}
	if *password == "" {
		fatal(fmt.Errorf("provide --password or ALPHADRIVE_PASSWORD"))
	}
	if len(*password) < 12 {
		fatal(fmt.Errorf("password must be at least 12 characters"))
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		fatal(err)
	}
	defer a.Close()
	if err := a.ResetPassword(context.Background(), *username, *password); err != nil {
		fatal(err)
	}
	slog.Info("password reset successfully", "username", *username)
}

func runBackup(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	configPath := fs.String("config", "", "path to JSON configuration")
	output := fs.String("output", "", "output backup archive path (.tar.gz)")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		fatal(err)
	}
	defer a.Close()
	if err := a.Backup(context.Background(), *output); err != nil {
		fatal(fmt.Errorf("backup failed: %w", err))
	}
	slog.Info("backup completed successfully", "output", *output)
}

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	configPath := fs.String("config", "", "path to JSON configuration")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		fatal(err)
	}
	defer a.Close()
	if err := a.Doctor(context.Background()); err != nil {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "AlphaDrive - Self-Hostable VPS Cloud Drive")
	fmt.Fprintln(os.Stderr, "Usage: alphadrive <command> [options]")
	fmt.Fprintln(os.Stderr, "\nCommands:")
	fmt.Fprintln(os.Stderr, "  serve            Start the AlphaDrive HTTP server")
	fmt.Fprintln(os.Stderr, "  create-user      Create a new user account (min 12 chars)")
	fmt.Fprintln(os.Stderr, "  reset-password   Reset password for an existing user (min 12 chars)")
	fmt.Fprintln(os.Stderr, "  backup           Generate hot, consistent backup of database and storage")
	fmt.Fprintln(os.Stderr, "  doctor           Run system integrity and diagnostics checks")
	fmt.Fprintln(os.Stderr, "  version          Print AlphaDrive version and build information")
}

func fatal(err error) {
	if err != nil {
		slog.Error("alphadrive failed", "error", err)
		os.Exit(1)
	}
}
