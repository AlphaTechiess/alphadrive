package app

import (
	"context"
	"fmt"
	"time"

	"github.com/AlphaTechiess/alphadrive/internal/auth"
	"github.com/AlphaTechiess/alphadrive/internal/backup"
	"github.com/AlphaTechiess/alphadrive/internal/config"
	"github.com/AlphaTechiess/alphadrive/internal/database"
	"github.com/AlphaTechiess/alphadrive/internal/doctor"
	"github.com/AlphaTechiess/alphadrive/internal/files"
	"github.com/AlphaTechiess/alphadrive/internal/httpserver"
	"github.com/AlphaTechiess/alphadrive/internal/shares"
)

type App struct {
	cfg          config.Config
	db           *database.DB
	fileService  *files.Service
	shareService *shares.Service
	server       *httpserver.Server
}

func New(cfg config.Config) (*App, error) {
	db, err := database.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	fs, err := files.New(db.DB, cfg.DataDir)
	if err != nil {
		db.Close()
		return nil, err
	}
	ss := shares.New(db.DB)
	srv := httpserver.New(cfg, db.DB, fs, ss)

	return &App{
		cfg:          cfg,
		db:           db,
		fileService:  fs,
		shareService: ss,
		server:       srv,
	}, nil
}

func (a *App) Close() error {
	return a.db.Close()
}

func (a *App) CreateUser(ctx context.Context, username, password string, admin bool) error {
	return a.CreateUserWithName(ctx, "", username, password, admin)
}

func (a *App) CreateUserWithName(ctx context.Context, name, username, password string, admin bool) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return a.server.CreateUserWithName(ctx, name, username, hash, admin)
}

func (a *App) ResetPassword(ctx context.Context, username, password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	res, err := a.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE username = ?`, hash, now, username)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("user %q not found", username)
	}
	// Invalidate active sessions upon password reset
	_, _ = a.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = (SELECT id FROM users WHERE username = ?)`, now, username)
	return nil
}

func (a *App) Doctor(ctx context.Context) error {
	if !doctor.Run(ctx, a.cfg, a.db.DB, nil) {
		return fmt.Errorf("diagnostics reported failures")
	}
	return nil
}

func (a *App) Backup(ctx context.Context, outputPath string) error {
	return backup.Create(ctx, a.db.DB, a.cfg.DataDir, outputPath)
}

func (a *App) Serve(ctx context.Context) error {
	if err := a.server.Serve(ctx); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
