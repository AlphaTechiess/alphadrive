package httpserver

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	assets "github.com/AlphaTechiess/alphadrive"
	"github.com/AlphaTechiess/alphadrive/internal/auth"
	"github.com/AlphaTechiess/alphadrive/internal/config"
	"github.com/AlphaTechiess/alphadrive/internal/files"
	"github.com/AlphaTechiess/alphadrive/internal/shares"
	"github.com/AlphaTechiess/alphadrive/internal/system"
	"github.com/AlphaTechiess/alphadrive/web"
)

const sessionCookie = "alphadrive_session"

type Server struct {
	cfg           config.Config
	db            *sql.DB
	files         *files.Service
	shares        *shares.Service
	templates     *template.Template
	uploadSem     chan struct{}
	loginAttempts *attempts
	shareAttempts *attempts
}
type principal struct {
	UserID, Username, Name string
	IsAdmin                bool
	CSRF                   string
}
type contextKey string

const principalKey contextKey = "principal"

func New(cfg config.Config, db *sql.DB, fileService *files.Service, shareService *shares.Service) *Server {
	t := template.Must(template.ParseFS(web.Files, "templates/*.html"))
	return &Server{
		cfg:           cfg,
		db:            db,
		files:         fileService,
		shares:        shareService,
		templates:     t,
		uploadSem:     make(chan struct{}, cfg.MaxConcurrentUpload),
		loginAttempts: newAttempts(),
		shareAttempts: newAttempts(),
	}
}

func (s *Server) CreateUser(ctx context.Context, username, passwordHash string, admin bool) error {
	return s.CreateUserWithName(ctx, "", username, passwordHash, admin)
}

func (s *Server) CreateUserWithName(ctx context.Context, name, username, passwordHash string, admin bool) error {
	username = strings.TrimSpace(username)
	name = strings.TrimSpace(name)
	if username == "" || len(username) > 64 {
		return fmt.Errorf("invalid username")
	}
	for _, r := range username {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return fmt.Errorf("invalid username")
		}
	}
	id, err := id()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	_, err = s.db.ExecContext(ctx, `INSERT INTO users(id,name,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, id, name, username, passwordHash, boolInt(admin), now, now)
	if err != nil {
		return err
	}
	if err := s.files.EnsureRoot(ctx, id); err != nil {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
		return err
	}
	return nil
}

func (s *Server) ResetPassword(ctx context.Context, username, newPassword string) error {
	username = strings.TrimSpace(username)
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	var userID string
	err = s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, now, userID); err != nil {
		return err
	}
	// Revoke all existing sessions for security
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		data, err := assets.Images.ReadFile("images/logo.png")
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler()))
	mux.HandleFunc("GET /setup", s.setupPage)
	mux.HandleFunc("POST /setup", s.setup)
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.require(s.csrf(s.logout)))
	mux.HandleFunc("GET /", s.require(s.dashboard))
	mux.HandleFunc("GET /trash", s.require(s.dashboard))
	mux.HandleFunc("GET /api/me", s.require(s.me))
	mux.HandleFunc("GET /api/nodes", s.require(s.listNodes))
	mux.HandleFunc("GET /api/nodes/{id}", s.require(s.getNode))
	mux.HandleFunc("GET /api/nodes/{id}/share", s.require(s.getNodeShare))
	mux.HandleFunc("GET /api/trash", s.require(s.listTrash))
	mux.HandleFunc("POST /api/trash/empty", s.require(s.csrf(s.emptyTrash)))
	mux.HandleFunc("DELETE /api/trash", s.require(s.csrf(s.emptyTrash)))
	mux.HandleFunc("POST /api/folders", s.require(s.csrf(s.createFolder)))
	mux.HandleFunc("POST /api/uploads", s.require(s.csrf(s.upload)))
	mux.HandleFunc("POST /api/nodes/trash", s.require(s.csrf(s.trashNodes)))
	mux.HandleFunc("POST /api/nodes/restore", s.require(s.csrf(s.restoreNodes)))
	mux.HandleFunc("POST /api/nodes/move", s.require(s.csrf(s.moveNodes)))
	mux.HandleFunc("DELETE /api/nodes", s.require(s.csrf(s.deleteNodes)))
	mux.HandleFunc("POST /api/nodes/download", s.require(s.csrf(s.downloadZip)))
	mux.HandleFunc("GET /api/files/{id}/download", s.require(s.download))
	mux.HandleFunc("GET /api/files/{id}/view", s.require(s.viewFile))

	// Authenticated Share Management APIs
	mux.HandleFunc("POST /api/shares", s.require(s.csrf(s.createShare)))
	mux.HandleFunc("GET /api/shares", s.require(s.listShares))
	mux.HandleFunc("DELETE /api/shares/{id}", s.require(s.csrf(s.revokeShare)))

	// Account Settings APIs
	mux.HandleFunc("POST /api/account/username", s.require(s.csrf(s.updateUsername)))
	mux.HandleFunc("POST /api/account/password", s.require(s.csrf(s.updatePassword)))
	mux.HandleFunc("POST /api/account/users", s.require(s.csrf(s.addUser)))

	// Public Share Routes
	mux.HandleFunc("GET /s/{slug}", s.publicSharePage)
	mux.HandleFunc("POST /s/{slug}/unlock", s.unlockShare)
	mux.HandleFunc("GET /s/{slug}/nodes", s.listSharedNodes)
	mux.HandleFunc("GET /s/{slug}/files/{id}/view", s.viewSharedFile)
	mux.HandleFunc("GET /s/{slug}/files/{id}/download", s.downloadSharedFile)
	mux.HandleFunc("POST /s/{slug}/download", s.downloadSharedZip)

	return s.recover(s.log(s.headers(mux)))
}

func (s *Server) Serve(ctx context.Context) error {
	server := &http.Server{Addr: s.cfg.ListenAddress, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	slog.Info("starting AlphaDrive", "address", s.cfg.ListenAddress)
	return server.ListenAndServe()
}

func staticHandler() http.Handler {
	static, _ := fs.Sub(web.Files, "static")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		if strings.HasPrefix(r.URL.Path, "images/") {
			http.FileServer(http.FS(assets.Images)).ServeHTTP(w, r)
			return
		}
		http.FileServer(http.FS(static)).ServeHTTP(w, r)
	})
}

func (s *Server) hasUsers(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	hasUsers, _ := s.hasUsers(r.Context())
	if hasUsers {
		if _, ok := s.loadPrincipal(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}
	s.renderSetup(w, r, "", "", "", http.StatusOK)
}

func (s *Server) renderSetup(w http.ResponseWriter, r *http.Request, errMsg, name, username string, status int) {
	raw := s.ensureLoginCSRFCookie(w, r)
	data := map[string]any{
		"CSRF":     raw,
		"Error":    errMsg,
		"Name":     name,
		"Username": username,
	}
	s.renderHTML(w, status, "setup.html", data)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderSetup(w, r, "Invalid setup request.", "", "", http.StatusBadRequest)
		return
	}
	if !validLoginCSRF(r) {
		s.renderSetup(w, r, "Your form expired. Please try again.", "", "", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if name == "" {
		s.renderSetup(w, r, "Please enter your name.", name, username, http.StatusBadRequest)
		return
	}
	if username == "" || len(username) < 3 || len(username) > 32 {
		s.renderSetup(w, r, "Username must be between 3 and 32 characters.", name, username, http.StatusBadRequest)
		return
	}
	for _, ch := range username {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.') {
			s.renderSetup(w, r, "Username contains invalid characters.", name, username, http.StatusBadRequest)
			return
		}
	}
	if len(password) < 8 {
		s.renderSetup(w, r, "Password must be at least 8 characters long.", name, username, http.StatusBadRequest)
		return
	}
	if confirmPassword != "" && password != confirmPassword {
		s.renderSetup(w, r, "Passwords do not match.", name, username, http.StatusBadRequest)
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback()

	var userCount int
	if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		internalError(w, r, err)
		return
	}
	if userCount > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		internalError(w, r, err)
		return
	}

	userID, err := id()
	if err != nil {
		internalError(w, r, err)
		return
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO users(id,name,username,password_hash,is_admin,created_at,updated_at,last_login_at) VALUES(?,?,?,?,1,?,?,?)`, userID, name, username, passwordHash, now.Unix(), now.Unix(), now.Unix())
	if err != nil {
		internalError(w, r, err)
		return
	}

	raw, tokenHash, err := auth.Token()
	if err != nil {
		internalError(w, r, err)
		return
	}
	sessionID, err := id()
	if err != nil {
		internalError(w, r, err)
		return
	}
	expires := now.Add(s.cfg.SessionMaxLifetime)
	_, err = tx.ExecContext(r.Context(), `INSERT INTO sessions(id,user_id,token_hash,csrf_secret,created_at,last_seen_at,expires_at) VALUES(?,?,?,?,?,?,?)`, sessionID, userID, tokenHash, tokenHash, now.Unix(), now.Unix(), expires.Unix())
	if err != nil {
		internalError(w, r, err)
		return
	}

	if err := tx.Commit(); err != nil {
		internalError(w, r, err)
		return
	}

	_ = s.files.EnsureRoot(r.Context(), userID)

	s.setSessionCookie(w, raw, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.loadPrincipal(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	hasUsers, _ := s.hasUsers(r.Context())
	if !hasUsers {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	s.renderLogin(w, r, "", http.StatusOK)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	hasUsers, _ := s.hasUsers(r.Context())
	if !hasUsers {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderLoginError(w, r, "Invalid sign-in request.", http.StatusBadRequest)
		return
	}
	if !validLoginCSRF(r) {
		s.renderLoginError(w, r, "Your form expired. Please try again.", http.StatusBadRequest)
		return
	}
	if !s.loginAttempts.Allow(clientIP(r)) {
		s.renderLoginError(w, r, "Too many attempts. Please try again later.", http.StatusTooManyRequests)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	var userID, hash string
	var disabled sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `SELECT id,password_hash,disabled_at FROM users WHERE username=?`, username).Scan(&userID, &hash, &disabled)
	if err != nil || disabled.Valid || !auth.VerifyPassword(hash, password) {
		s.loginAttempts.Fail(clientIP(r))
		s.renderLoginError(w, r, "Invalid username or password.", http.StatusUnauthorized)
		return
	}
	raw, tokenHash, err := auth.Token()
	if err != nil {
		internalError(w, r, err)
		return
	}
	sessionID, err := id()
	if err != nil {
		internalError(w, r, err)
		return
	}
	now := time.Now().UTC()
	expires := now.Add(s.cfg.SessionMaxLifetime)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO sessions(id,user_id,token_hash,csrf_secret,created_at,last_seen_at,expires_at) VALUES(?,?,?,?,?,?,?)`, sessionID, userID, tokenHash, tokenHash, now.Unix(), now.Unix(), expires.Unix())
	if err != nil {
		internalError(w, r, err)
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `UPDATE users SET last_login_at=? WHERE id=?`, now.Unix(), userID)
	s.loginAttempts.Success(clientIP(r))
	s.setSessionCookie(w, raw, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	_, _ = s.db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE user_id=? AND token_hash=?`, time.Now().Unix(), p.UserID, tokenHash(r))
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	s.render(w, "app.html", p)
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	alphadriveUsed, err := s.files.StorageUsage(r.Context(), p.UserID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	diskStats, err := system.GetDiskStats(s.cfg.DataDir)
	detectionAvailable := (err == nil)

	resp := map[string]any{
		"username":                    p.Username,
		"name":                        p.Name,
		"is_admin":                    p.IsAdmin,
		"root_id":                     files.RootID(p.UserID),
		"alphadrive_used_bytes":       alphadriveUsed,
		"used_bytes":                  alphadriveUsed,
		"storage_detection_available": detectionAvailable,
	}

	if detectionAvailable {
		resp["filesystem_total_bytes"] = diskStats.TotalBytes
		resp["filesystem_used_bytes"] = diskStats.UsedBytes
		resp["filesystem_free_bytes"] = diskStats.FreeBytes
		resp["server_total"] = diskStats.TotalBytes
		resp["server_used"] = diskStats.UsedBytes
		resp["server_free"] = diskStats.FreeBytes
	} else {
		resp["filesystem_total_bytes"] = nil
		resp["filesystem_used_bytes"] = nil
		resp["filesystem_free_bytes"] = nil
		resp["server_total"] = nil
		resp["server_used"] = nil
		resp["server_free"] = nil
	}

	jsonResponse(w, http.StatusOK, resp)
}
func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query != "" {
		parentID := r.URL.Query().Get("parent_id")
		nodes, err := s.files.Search(r.Context(), p.UserID, query, parentID)
		if errors.Is(err, files.ErrNotFound) {
			apiError(w, http.StatusNotFound, "not_found", "Folder not found.")
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		jsonResponse(w, http.StatusOK, map[string]any{
			"nodes":       nodes,
			"query":       query,
			"breadcrumbs": []files.Node{{ID: files.RootID(p.UserID), Name: "Search results", Kind: "folder"}},
		})
		return
	}

	parentID := r.URL.Query().Get("parent_id")
	if parentID == "" {
		parentID = files.RootID(p.UserID)
	}
	nodes, err := s.files.List(r.Context(), p.UserID, parentID)
	if errors.Is(err, files.ErrNotFound) {
		apiError(w, http.StatusNotFound, "not_found", "Folder not found.")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	crumbs, err := s.files.Breadcrumbs(r.Context(), p.UserID, parentID)
	if err != nil {
		crumbs = []files.Node{{ID: files.RootID(p.UserID), Name: "My drive", Kind: "folder"}}
	}
	current, _ := s.files.Get(r.Context(), p.UserID, parentID)

	jsonResponse(w, http.StatusOK, map[string]any{
		"nodes":       nodes,
		"parent_id":   parentID,
		"current":     current,
		"breadcrumbs": crumbs,
	})
}
func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	node, err := s.files.Get(r.Context(), p.UserID, r.PathValue("id"))
	if errors.Is(err, files.ErrNotFound) {
		apiError(w, http.StatusNotFound, "not_found", "Item not found.")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, node)
}
func (s *Server) listTrash(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	nodes, err := s.files.ListTrash(r.Context(), p.UserID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"nodes": nodes})
}
func (s *Server) trashNodes(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := s.files.Trash(r.Context(), p.UserID, input.IDs); err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}
func (s *Server) restoreNodes(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := s.files.Restore(r.Context(), p.UserID, input.IDs); err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}
func (s *Server) moveNodes(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		TargetID string   `json:"target_id"`
		IDs      []string `json:"ids"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.IDs) == 0 {
		apiError(w, http.StatusBadRequest, "invalid_input", "No items specified to move.")
		return
	}
	if err := s.files.Move(r.Context(), p.UserID, input.TargetID, input.IDs); err != nil {
		if errors.Is(err, files.ErrNotFound) {
			apiError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		if strings.Contains(err.Error(), "cannot move") || strings.Contains(err.Error(), "target must be") {
			apiError(w, http.StatusBadRequest, "invalid_move", err.Error())
			return
		}
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"status": "ok", "moved_count": len(input.IDs)})
}
func (s *Server) deleteNodes(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := s.files.DeletePermanently(r.Context(), p.UserID, input.IDs); err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}
func (s *Server) emptyTrash(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	if err := s.files.EmptyTrash(r.Context(), p.UserID); err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}
func (s *Server) createFolder(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	nodeID, err := id()
	if err != nil {
		internalError(w, r, err)
		return
	}
	n, err := s.files.CreateFolder(r.Context(), p.UserID, input.ParentID, input.Name, nodeID)
	if errors.Is(err, files.ErrNotFound) {
		apiError(w, http.StatusNotFound, "not_found", "Folder not found.")
		return
	}
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid_folder", "Unable to create that folder.")
		return
	}
	jsonResponse(w, http.StatusCreated, n)
}
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	select {
	case s.uploadSem <- struct{}{}:
		defer func() { <-s.uploadSem }()
	default:
		apiError(w, http.StatusTooManyRequests, "uploads_busy", "Too many uploads in progress.")
		return
	}
	if s.cfg.MaxUploadBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes+1024*1024)
	}
	if err := r.ParseMultipartForm(32 * 1024 * 1024); err != nil {
		apiError(w, http.StatusBadRequest, "invalid_upload", "Unable to read upload.")
		return
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		apiError(w, http.StatusBadRequest, "missing_file", "Choose a file to upload.")
		return
	}
	defer f.Close()
	nodeID, err := id()
	if err != nil {
		internalError(w, r, err)
		return
	}
	n, err := s.files.Upload(r.Context(), p.UserID, r.FormValue("parent_id"), h.Filename, nodeID, f, s.cfg.MaxUploadBytes)
	if errors.Is(err, files.ErrNotFound) {
		apiError(w, http.StatusNotFound, "not_found", "Folder not found.")
		return
	}
	if err != nil {
		apiError(w, http.StatusBadRequest, "upload_failed", "Unable to upload that file.")
		return
	}
	jsonResponse(w, http.StatusCreated, n)
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	f, n, err := s.files.OpenDownload(r.Context(), p.UserID, r.PathValue("id"))
	if errors.Is(err, files.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", n.MIMEType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeDisposition(n.Name)+`"`)
	http.ServeContent(w, r, n.Name, n.UpdatedAt, f)
}
func (s *Server) viewFile(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	f, n, err := s.files.OpenDownload(r.Context(), p.UserID, r.PathValue("id"))
	if errors.Is(err, files.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer f.Close()
	mimeType := n.MIMEType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", `inline; filename="`+safeDisposition(n.Name)+`"`)
	http.ServeContent(w, r, n.Name, n.UpdatedAt, f)
}
func (s *Server) downloadZip(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.IDs) == 0 {
		apiError(w, http.StatusBadRequest, "invalid_request", "No items selected.")
		return
	}
	filename := "alphadrive-archive.zip"
	if len(input.IDs) == 1 {
		if node, err := s.files.Get(r.Context(), p.UserID, input.IDs[0]); err == nil {
			filename = safeDisposition(node.Name) + ".zip"
		}
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	if err := s.files.WriteZip(r.Context(), p.UserID, input.IDs, w); err != nil {
		slog.Error("zip download failed", "error", err)
	}
}

func (s *Server) require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.loadPrincipal(r)
		if !ok {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				apiError(w, http.StatusUnauthorized, "unauthenticated", "Please sign in.")
			} else {
				hasUsers, _ := s.hasUsers(r.Context())
				if !hasUsers {
					http.Redirect(w, r, "/setup", http.StatusSeeOther)
				} else {
					http.Redirect(w, r, "/login", http.StatusSeeOther)
				}
			}
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	}
}
func (s *Server) csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := s.principal(r)
		if r.Header.Get("X-CSRF-Token") != p.CSRF && r.FormValue("csrf") != p.CSRF {
			apiError(w, http.StatusForbidden, "csrf_failed", "Your request could not be verified.")
			return
		}
		next(w, r)
	}
}
func (s *Server) loadPrincipal(r *http.Request) (principal, bool) {
	hash := tokenHash(r)
	if hash == nil {
		return principal{}, false
	}
	var p principal
	var name sql.NullString
	var isAdmin int
	var csrfSecret []byte
	var expires, revoked, lastSeen sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,COALESCE(u.name, ''),u.is_admin,s.csrf_secret,s.expires_at,s.revoked_at,s.last_seen_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND u.disabled_at IS NULL`, hash).Scan(&p.UserID, &p.Username, &name, &isAdmin, &csrfSecret, &expires, &revoked, &lastSeen)
	now := time.Now()
	if err != nil || revoked.Valid || expires.Int64 < now.Unix() || lastSeen.Int64 < now.Add(-s.cfg.SessionIdleTimeout).Unix() {
		return principal{}, false
	}
	if lastSeen.Int64 < now.Add(-5*time.Minute).Unix() {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE sessions SET last_seen_at=? WHERE token_hash=?`, now.Unix(), hash)
	}
	p.Name = name.String
	p.IsAdmin = (isAdmin == 1)
	p.CSRF = auth.CSRF(csrfSecret)
	return p, true
}

func (s *Server) updateUsername(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		NewUsername string `json:"new_username"`
		Username    string `json:"username"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	newUsername := strings.TrimSpace(input.NewUsername)
	if newUsername == "" {
		newUsername = strings.TrimSpace(input.Username)
	}
	if newUsername == "" || len(newUsername) < 3 || len(newUsername) > 32 {
		apiError(w, http.StatusBadRequest, "invalid_username", "Username must be between 3 and 32 characters.")
		return
	}
	for _, ch := range newUsername {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.') {
			apiError(w, http.StatusBadRequest, "invalid_username", "Username contains invalid characters.")
			return
		}
	}
	now := time.Now().UTC().Unix()
	_, err := s.db.ExecContext(r.Context(), `UPDATE users SET username=?, updated_at=? WHERE id=?`, newUsername, now, p.UserID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apiError(w, http.StatusConflict, "username_taken", "That username is already taken.")
			return
		}
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"status": "ok", "username": newUsername})
}

func (s *Server) updatePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.NewPassword) < 8 {
		apiError(w, http.StatusBadRequest, "invalid_password", "New password must be at least 8 characters long.")
		return
	}

	var currentHash string
	err := s.db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=?`, p.UserID).Scan(&currentHash)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if !auth.VerifyPassword(currentHash, input.CurrentPassword) {
		apiError(w, http.StatusUnauthorized, "invalid_current_password", "Current password is incorrect.")
		return
	}

	newHash, err := auth.HashPassword(input.NewPassword)
	if err != nil {
		internalError(w, r, err)
		return
	}

	now := time.Now().UTC().Unix()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(), `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, newHash, now, p.UserID); err != nil {
		internalError(w, r, err)
		return
	}

	// Revoke other active sessions for this user except current session
	currentHashToken := tokenHash(r)
	if currentHashToken != nil {
		_, _ = tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE user_id=? AND token_hash!=?`, now, p.UserID, currentHashToken)
	}

	if err := tx.Commit(); err != nil {
		internalError(w, r, err)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) addUser(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	if !p.IsAdmin {
		apiError(w, http.StatusForbidden, "forbidden", "Only administrators can add new users.")
		return
	}
	var input struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	name := strings.TrimSpace(input.Name)
	username := strings.TrimSpace(input.Username)
	password := input.Password

	if username == "" || len(username) < 3 || len(username) > 32 {
		apiError(w, http.StatusBadRequest, "invalid_username", "Username must be between 3 and 32 characters.")
		return
	}
	for _, ch := range username {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.') {
			apiError(w, http.StatusBadRequest, "invalid_username", "Username contains invalid characters.")
			return
		}
	}
	if len(password) < 8 {
		apiError(w, http.StatusBadRequest, "invalid_password", "Password must be at least 8 characters long.")
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		internalError(w, r, err)
		return
	}

	userID, err := id()
	if err != nil {
		internalError(w, r, err)
		return
	}

	now := time.Now().UTC().Unix()
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO users(id,name,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, userID, name, username, hash, boolInt(input.IsAdmin), now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apiError(w, http.StatusConflict, "username_taken", "That username is already taken.")
			return
		}
		internalError(w, r, err)
		return
	}

	if err := s.files.EnsureRoot(r.Context(), userID); err != nil {
		slog.Error("failed to ensure root for new user", "error", err)
	}

	jsonResponse(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"user": map[string]any{
			"id":       userID,
			"name":     name,
			"username": username,
			"is_admin": input.IsAdmin,
		},
	})
}
func (s *Server) principal(r *http.Request) (principal, bool) {
	p, ok := r.Context().Value(principalKey).(principal)
	return p, ok
}
func tokenHash(r *http.Request) []byte {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	_, h, err := auth.TokenFromRaw(c.Value)
	_ = err
	return h
}
func (s *Server) setSessionCookie(w http.ResponseWriter, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: raw, Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (s *Server) renderHTML(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status > 0 {
		w.WriteHeader(status)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("template error", "error", err, "template", name)
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	s.renderHTML(w, http.StatusOK, name, data)
}
func (s *Server) ensureLoginCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	var raw string
	if c, err := r.Cookie("alphadrive_login_csrf"); err == nil && c.Value != "" {
		if _, _, err := auth.TokenFromRaw(c.Value); err == nil {
			raw = c.Value
		}
	}
	if raw == "" {
		var err error
		raw, _, err = auth.Token()
		if err == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "alphadrive_login_csrf",
				Value:    raw,
				Path:     "/",
				HttpOnly: true,
				Secure:   s.cfg.SecureCookies,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   600,
			})
		}
	}
	return raw
}
func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, message string, status int) {
	raw := s.ensureLoginCSRFCookie(w, r)
	s.renderHTML(w, status, "login.html", map[string]string{"CSRF": raw, "Error": message})
}
func (s *Server) renderLoginError(w http.ResponseWriter, r *http.Request, message string, status int) {
	s.renderLogin(w, r, message, status)
}
func id() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func safeDisposition(s string) string {
	return strings.NewReplacer("\"", "'", "\\", "_", "\r", "", "\n", "").Replace(s)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		apiError(w, http.StatusBadRequest, "invalid_request", "Invalid request.")
		return false
	}
	return true
}
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, message string) {
	jsonResponse(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "path", r.URL.Path, "error", err)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		apiError(w, http.StatusInternalServerError, "internal_error", "Something went wrong.")
	} else {
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
	}
}
func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; script-src 'self' 'unsafe-inline'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}
func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "value", v)
				http.Error(w, "Something went wrong.", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type attempts struct {
	mu     sync.Mutex
	values map[string]attempt
}
type attempt struct {
	count int
	until time.Time
}

func newAttempts() *attempts { return &attempts{values: map[string]attempt{}} }
func (a *attempts) Allow(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.values[key].until.After(time.Now())
}
func (a *attempts) Fail(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.values[key]
	v.count++
	if v.count >= 5 {
		v.until = time.Now().Add(5 * time.Minute)
		v.count = 0
	}
	a.values[key] = v
}
func (a *attempts) Success(key string) { a.mu.Lock(); defer a.mu.Unlock(); delete(a.values, key) }
func clientIP(r *http.Request) string  { return r.RemoteAddr }
func validLoginCSRF(r *http.Request) bool {
	c, err := r.Cookie("alphadrive_login_csrf")
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(r.FormValue("csrf"))) == 1
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	var input struct {
		NodeID     string `json:"node_id"`
		Slug       string `json:"slug"`
		CustomSlug string `json:"custom_slug"`
		Password   string `json:"password"`
		Expiry     string `json:"expiry"`
		ExpiresIn  string `json:"expires_in"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.NodeID == "" {
		apiError(w, http.StatusBadRequest, "invalid_input", "node_id is required")
		return
	}

	slug := input.Slug
	if slug == "" {
		slug = input.CustomSlug
	}

	expiryStr := input.Expiry
	if expiryStr == "" {
		expiryStr = input.ExpiresIn
	}

	expiresAt, err := shares.ParseExpiry(expiryStr)
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid_expiry", err.Error())
		return
	}

	sh, err := s.shares.Create(r.Context(), p.UserID, input.NodeID, slug, input.Password, expiresAt)
	if errors.Is(err, shares.ErrSlugTaken) {
		apiError(w, http.StatusConflict, "slug_taken", "That link slug is already taken.")
		return
	}
	if errors.Is(err, shares.ErrInvalidSlug) {
		apiError(w, http.StatusBadRequest, "invalid_slug", err.Error())
		return
	}
	if errors.Is(err, shares.ErrInvalidExpiry) {
		apiError(w, http.StatusBadRequest, "invalid_expiry", err.Error())
		return
	}
	if errors.Is(err, files.ErrNotFound) {
		apiError(w, http.StatusNotFound, "not_found", "Item not found.")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "password") {
			apiError(w, http.StatusBadRequest, "invalid_password", err.Error())
			return
		}
		internalError(w, r, err)
		return
	}

	baseURL := s.cfg.PublicBaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s", s.cfg.ListenAddress)
	}
	publicURL := fmt.Sprintf("%s/s/%s", baseURL, sh.Slug)

	jsonResponse(w, http.StatusCreated, map[string]any{
		"id":           sh.ID,
		"node_id":      sh.NodeID,
		"slug":         sh.Slug,
		"has_password": sh.HasPassword,
		"expires_at":   sh.ExpiresAt,
		"view_count":   sh.ViewCount,
		"created_at":   sh.CreatedAt,
		"updated_at":   sh.UpdatedAt,
		"public_url":   publicURL,
		"share":        sh,
	})
}

func (s *Server) listShares(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	list, err := s.shares.ListByUser(r.Context(), p.UserID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"shares": list})
}

func (s *Server) getNodeShare(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	nodeID := r.PathValue("id")
	sh, err := s.shares.GetByNode(r.Context(), p.UserID, nodeID)
	if errors.Is(err, shares.ErrNotFound) {
		apiError(w, http.StatusNotFound, "not_found", "No active share for this item.")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	baseURL := s.cfg.PublicBaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s", s.cfg.ListenAddress)
	}
	publicURL := fmt.Sprintf("%s/s/%s", baseURL, sh.Slug)
	jsonResponse(w, http.StatusOK, map[string]any{
		"id":           sh.ID,
		"node_id":      sh.NodeID,
		"slug":         sh.Slug,
		"has_password": sh.HasPassword,
		"expires_at":   sh.ExpiresAt,
		"view_count":   sh.ViewCount,
		"created_at":   sh.CreatedAt,
		"updated_at":   sh.UpdatedAt,
		"public_url":   publicURL,
		"share":        sh,
	})
}

func (s *Server) revokeShare(w http.ResponseWriter, r *http.Request) {
	p, _ := s.principal(r)
	shareID := r.PathValue("id")
	if err := s.shares.Revoke(r.Context(), p.UserID, shareID); err != nil {
		if errors.Is(err, shares.ErrNotFound) {
			apiError(w, http.StatusNotFound, "not_found", "Share not found.")
			return
		}
		internalError(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) shareCookieName(slug string) string {
	return "alphadrive_share_" + slug
}

func (s *Server) hasShareAccess(r *http.Request, sh *shares.Share) bool {
	if !sh.HasPassword {
		return true
	}
	c, err := r.Cookie(s.shareCookieName(sh.Slug))
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return false
	}
	ok, err := s.shares.VerifyGrant(r.Context(), sh.ID, c.Value)
	if err != nil {
		return false
	}
	return ok
}

func (s *Server) grantShareAccess(w http.ResponseWriter, r *http.Request, sh *shares.Share) error {
	ttl := 24 * time.Hour
	rawToken, err := s.shares.CreateGrant(r.Context(), sh.ID, ttl, sh.ExpiresAt)
	if err != nil {
		return err
	}
	maxAge := int(ttl.Seconds())
	if sh.ExpiresAt != nil {
		rem := int(time.Until(*sh.ExpiresAt).Seconds())
		if rem < maxAge && rem > 0 {
			maxAge = rem
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.shareCookieName(sh.Slug),
		Value:    rawToken,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

type publicData struct {
	State         string
	Share         *shares.Share
	ErrorMessage  string
	FileIcon      string
	FormattedSize string
	IsImage       bool
	IsVideo       bool
	IsAudio       bool
	IsPDF         bool
	IsText        bool
	TextContent   string
}

func (s *Server) renderPublic(w http.ResponseWriter, r *http.Request, state string, sh *shares.Share, errMsg string, status int) {
	data := publicData{
		State:        state,
		Share:        sh,
		ErrorMessage: errMsg,
	}
	if sh != nil && state == "file" {
		data.FileIcon = iconForFilename(sh.NodeName, sh.NodeKind)
		data.FormattedSize = formatByteSize(sh.NodeSize)
		mime := strings.ToLower(sh.NodeMIME)
		name := strings.ToLower(sh.NodeName)
		if strings.HasPrefix(mime, "image/") || isImgExt(name) {
			data.IsImage = true
		} else if strings.HasPrefix(mime, "video/") || isVideoExt(name) {
			data.IsVideo = true
		} else if strings.HasPrefix(mime, "audio/") || isAudioExt(name) {
			data.IsAudio = true
		} else if mime == "application/pdf" || strings.HasSuffix(name, ".pdf") {
			data.IsPDF = true
		} else if strings.HasPrefix(mime, "text/") || isTextExt(name) {
			data.IsText = true
			if rc, _, err := s.files.OpenDownload(r.Context(), sh.UserID, sh.NodeID); err == nil {
				buf := make([]byte, 65536)
				n, _ := io.ReadFull(rc, buf)
				rc.Close()
				data.TextContent = string(buf[:n])
			}
		}
	}

	s.renderHTML(w, status, "public.html", data)
}

func (s *Server) publicSharePage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sh, err := s.shares.GetBySlug(r.Context(), slug)
	if errors.Is(err, shares.ErrNotFound) {
		s.renderPublic(w, r, "error", nil, "Share link not found", http.StatusNotFound)
		return
	}
	if errors.Is(err, shares.ErrExpired) {
		s.renderPublic(w, r, "error", nil, "This link has expired", http.StatusGone)
		return
	}
	if errors.Is(err, shares.ErrRevoked) {
		s.renderPublic(w, r, "error", nil, "This link has been revoked", http.StatusGone)
		return
	}
	if errors.Is(err, shares.ErrNodeTrashed) {
		s.renderPublic(w, r, "error", nil, "This shared item is no longer available", http.StatusNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	_ = s.shares.IncrementViews(r.Context(), sh.ID)

	if sh.HasPassword && !s.hasShareAccess(r, sh) {
		s.renderPublic(w, r, "password", sh, "", http.StatusOK)
		return
	}

	if sh.NodeKind == "file" {
		s.renderPublic(w, r, "file", sh, "", http.StatusOK)
		return
	}

	s.renderPublic(w, r, "folder", sh, "", http.StatusOK)
}

func (s *Server) unlockShare(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sh, err := s.shares.GetBySlug(r.Context(), slug)
	if err != nil {
		http.Redirect(w, r, "/s/"+slug, http.StatusSeeOther)
		return
	}

	if !s.shareAttempts.Allow(clientIP(r)) {
		s.renderPublic(w, r, "password", sh, "Too many attempts. Please wait 5 minutes.", http.StatusTooManyRequests)
		return
	}

	_ = r.ParseForm()
	pass := r.FormValue("password")
	if pass == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		pass = body.Password
	}

	if !s.shares.VerifyPassword(sh, pass) {
		s.shareAttempts.Fail(clientIP(r))
		if strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			apiError(w, http.StatusUnauthorized, "invalid_password", "Incorrect password.")
			return
		}
		s.renderPublic(w, r, "password", sh, "Incorrect password. Please try again.", http.StatusUnauthorized)
		return
	}

	s.shareAttempts.Success(clientIP(r))
	if err := s.grantShareAccess(w, r, sh); err != nil {
		internalError(w, r, err)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	http.Redirect(w, r, "/s/"+slug, http.StatusSeeOther)
}

func (s *Server) listSharedNodes(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sh, err := s.shares.GetBySlug(r.Context(), slug)
	if err != nil {
		apiError(w, http.StatusNotFound, "not_found", "Share not found.")
		return
	}
	if sh.HasPassword && !s.hasShareAccess(r, sh) {
		apiError(w, http.StatusUnauthorized, "unauthorized", "Password required.")
		return
	}

	folderID := r.URL.Query().Get("folder_id")
	if folderID == "" {
		folderID = sh.NodeID
	}

	nodesList, err := s.shares.ListSharedChildren(r.Context(), sh.NodeID, folderID)
	if err != nil {
		apiError(w, http.StatusForbidden, "forbidden", "Access denied.")
		return
	}

	crumbs, err := s.shares.SharedBreadcrumbs(r.Context(), sh.NodeID, folderID)
	if err != nil {
		crumbs = []files.Node{{ID: sh.NodeID, Name: sh.NodeName, Kind: "folder"}}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"nodes":       nodesList,
		"breadcrumbs": crumbs,
		"folder_id":   folderID,
	})
}

func (s *Server) viewSharedFile(w http.ResponseWriter, r *http.Request) {
	s.serveSharedFile(w, r, true)
}

func (s *Server) downloadSharedFile(w http.ResponseWriter, r *http.Request) {
	s.serveSharedFile(w, r, false)
}

func (s *Server) serveSharedFile(w http.ResponseWriter, r *http.Request, inline bool) {
	slug := r.PathValue("slug")
	nodeID := r.PathValue("id")
	sh, err := s.shares.GetBySlug(r.Context(), slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if sh.HasPassword && !s.hasShareAccess(r, sh) {
		http.Error(w, "Password required", http.StatusUnauthorized)
		return
	}

	ok, err := s.shares.VerifyDescendant(r.Context(), sh.NodeID, nodeID)
	if err != nil || !ok {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	f, n, err := s.files.OpenDownload(r.Context(), sh.UserID, nodeID)
	if errors.Is(err, files.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	defer f.Close()

	mimeType := n.MIMEType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	disp := "attachment"
	if inline {
		disp = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, safeDisposition(n.Name)))
	http.ServeContent(w, r, n.Name, n.UpdatedAt, f)
}

func (s *Server) downloadSharedZip(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	sh, err := s.shares.GetBySlug(r.Context(), slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if sh.HasPassword && !s.hasShareAccess(r, sh) {
		http.Error(w, "Password required", http.StatusUnauthorized)
		return
	}

	var input struct {
		FolderID string `json:"folder_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	targetFolderID := input.FolderID
	if targetFolderID == "" {
		targetFolderID = sh.NodeID
	}

	ok, err := s.shares.VerifyDescendant(r.Context(), sh.NodeID, targetFolderID)
	if err != nil || !ok {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	zipName := sh.NodeName + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeDisposition(zipName)))

	zw := zip.NewWriter(w)
	defer zw.Close()

	var count int
	var totalBytes int64
	const maxFiles = 5000
	const maxBytes = 5 * 1024 * 1024 * 1024 // 5GB

	var walk func(folderID, relPath string, depth int) error
	walk = func(folderID, relPath string, depth int) error {
		if depth > 50 || count >= maxFiles || totalBytes >= maxBytes {
			return nil
		}
		children, err := s.shares.ListSharedChildren(r.Context(), sh.NodeID, folderID)
		if err != nil {
			return err
		}
		for _, child := range children {
			if count >= maxFiles || totalBytes >= maxBytes {
				break
			}
			childPath := filepath.Join(relPath, child.Name)
			if child.Kind == "folder" {
				if err := walk(child.ID, childPath, depth+1); err != nil {
					return err
				}
			} else {
				count++
				totalBytes += child.Size
				rc, _, err := s.files.OpenDownload(r.Context(), sh.UserID, child.ID)
				if err != nil {
					continue
				}
				fw, err := zw.Create(filepath.ToSlash(childPath))
				if err == nil {
					_, _ = io.Copy(fw, rc)
				}
				rc.Close()
			}
		}
		return nil
	}

	_ = walk(targetFolderID, "", 0)
}

func iconForFilename(name, kind string) string {
	if kind == "folder" {
		return "folder.svg"
	}
	n := strings.ToLower(name)
	if isImgExt(n) {
		return "image.svg"
	}
	if isVideoExt(n) {
		return "video.svg"
	}
	if isAudioExt(n) {
		return "audio.svg"
	}
	if strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".tar") || strings.HasSuffix(n, ".gz") {
		return "zip.svg"
	}
	return "files.svg"
}

func formatByteSize(b int64) string {
	if b == 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

func isImgExt(n string) bool {
	return strings.HasSuffix(n, ".png") || strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".jpeg") ||
		strings.HasSuffix(n, ".gif") || strings.HasSuffix(n, ".webp") || strings.HasSuffix(n, ".svg") || strings.HasSuffix(n, ".ico")
}

func isVideoExt(n string) bool {
	return strings.HasSuffix(n, ".mp4") || strings.HasSuffix(n, ".webm") || strings.HasSuffix(n, ".mkv") || strings.HasSuffix(n, ".mov")
}

func isAudioExt(n string) bool {
	return strings.HasSuffix(n, ".mp3") || strings.HasSuffix(n, ".wav") || strings.HasSuffix(n, ".ogg") || strings.HasSuffix(n, ".m4a") || strings.HasSuffix(n, ".flac")
}

func isTextExt(n string) bool {
	return strings.HasSuffix(n, ".txt") || strings.HasSuffix(n, ".md") || strings.HasSuffix(n, ".json") ||
		strings.HasSuffix(n, ".js") || strings.HasSuffix(n, ".ts") || strings.HasSuffix(n, ".css") ||
		strings.HasSuffix(n, ".html") || strings.HasSuffix(n, ".go") || strings.HasSuffix(n, ".py") ||
		strings.HasSuffix(n, ".log") || strings.HasSuffix(n, ".csv") || strings.HasSuffix(n, ".xml") || strings.HasSuffix(n, ".sql")
}
