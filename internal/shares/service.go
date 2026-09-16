package shares

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlphaTechiess/alphadrive/internal/auth"
	"github.com/AlphaTechiess/alphadrive/internal/files"
)

var (
	ErrNotFound       = errors.New("share not found")
	ErrExpired        = errors.New("share has expired")
	ErrRevoked        = errors.New("share has been revoked")
	ErrNodeTrashed    = errors.New("shared content is in trash")
	ErrInvalidSlug    = errors.New("invalid share slug")
	ErrSlugTaken      = errors.New("share slug already in use")
	ErrUnauthorized   = errors.New("unauthorized share action")
	ErrPasswordNeeded = errors.New("password required to access share")
	ErrWrongPassword  = errors.New("incorrect share password")
	ErrAccessDenied   = errors.New("access denied to requested item")
)

var reservedSlugs = map[string]bool{
	"api":       true,
	"static":    true,
	"login":     true,
	"logout":    true,
	"healthz":   true,
	"admin":     true,
	"share":     true,
	"shares":    true,
	"s":         true,
	"files":     true,
	"download":  true,
	"dashboard": true,
	"auth":      true,
}

type Share struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	NodeID       string     `json:"node_id"`
	Slug         string     `json:"slug"`
	HasPassword  bool       `json:"has_password"`
	PasswordHash *string    `json:"-"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	ViewCount    int64      `json:"view_count"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	NodeName     string     `json:"node_name,omitempty"`
	NodeKind     string     `json:"node_kind,omitempty"`
	NodeSize     int64      `json:"node_size,omitempty"`
	NodeMIME     string     `json:"node_mime,omitempty"`
}

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service {
	return &Service{db: db}
}

func ValidateSlug(slug string) error {
	slug = strings.TrimSpace(slug)
	if len(slug) < 3 || len(slug) > 64 {
		return fmt.Errorf("%w: length must be between 3 and 64 characters", ErrInvalidSlug)
	}
	if reservedSlugs[strings.ToLower(slug)] {
		return fmt.Errorf("%w: '%s' is a reserved keyword", ErrInvalidSlug, slug)
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("%w: only letters, numbers, hyphens and underscores allowed", ErrInvalidSlug)
		}
	}
	return nil
}

func GenerateSlug() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	slug := base64.RawURLEncoding.EncodeToString(b)
	slug = strings.ReplaceAll(slug, "-", "")
	slug = strings.ReplaceAll(slug, "_", "")
	if len(slug) > 10 {
		slug = slug[:10]
	}
	return strings.ToLower(slug), nil
}

func (s *Service) Create(ctx context.Context, userID, nodeID, customSlug, password string, expiresAt *time.Time) (*Share, error) {
	// Verify node exists, belongs to user, and is not in trash
	var nodeName, nodeKind, nodeMime string
	var nodeSize int64
	var trashedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT name, kind, size_bytes, coalesce(mime_type, ''), trashed_at FROM nodes WHERE id=? AND user_id=?`, nodeID, userID).
		Scan(&nodeName, &nodeKind, &nodeSize, &nodeMime, &trashedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, files.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if trashedAt.Valid {
		return nil, ErrNodeTrashed
	}

	slug := strings.TrimSpace(customSlug)
	if slug != "" {
		if err := ValidateSlug(slug); err != nil {
			return nil, err
		}
	} else {
		for attempts := 0; attempts < 5; attempts++ {
			gen, err := GenerateSlug()
			if err != nil {
				return nil, err
			}
			var exists int
			_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM shares WHERE slug=?`, gen).Scan(&exists)
			if exists == 0 {
				slug = gen
				break
			}
		}
		if slug == "" {
			return nil, fmt.Errorf("failed to generate unique share slug")
		}
	}

	// Check if slug is already taken
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM shares WHERE slug=? AND revoked_at IS NULL`, slug).Scan(&count); err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrSlugTaken
	}

	var pwdHash *string
	if strings.TrimSpace(password) != "" {
		h, err := auth.HashSharePassword(password)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		pwdHash = &h
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	shareID := "share_" + base64.RawURLEncoding.EncodeToString(b)
	now := time.Now().UTC()
	var expUnix sql.NullInt64
	if expiresAt != nil && !expiresAt.IsZero() {
		expUnix = sql.NullInt64{Int64: expiresAt.Unix(), Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO shares(id, user_id, node_id, slug, password_hash, expires_at, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)
	`, shareID, userID, nodeID, slug, pwdHash, expUnix, now.Unix(), now.Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrSlugTaken
		}
		return nil, err
	}

	return &Share{
		ID:          shareID,
		UserID:      userID,
		NodeID:      nodeID,
		Slug:        slug,
		HasPassword: pwdHash != nil,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
		NodeName:    nodeName,
		NodeKind:    nodeKind,
		NodeSize:    nodeSize,
		NodeMIME:    nodeMime,
	}, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (*Share, error) {
	slug = strings.TrimSpace(slug)
	var sh Share
	var pwdHash sql.NullString
	var expUnix sql.NullInt64
	var revokedUnix sql.NullInt64
	var createdUnix, updatedUnix int64
	var trashedAt sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.node_id, s.slug, s.password_hash, s.expires_at, s.view_count,
		       s.created_at, s.updated_at, s.revoked_at,
		       n.name, n.kind, n.size_bytes, coalesce(n.mime_type, ''), n.trashed_at
		FROM shares s
		JOIN nodes n ON s.node_id = n.id
		WHERE s.slug=?
	`, slug).Scan(
		&sh.ID, &sh.UserID, &sh.NodeID, &sh.Slug, &pwdHash, &expUnix, &sh.ViewCount,
		&createdUnix, &updatedUnix, &revokedUnix,
		&sh.NodeName, &sh.NodeKind, &sh.NodeSize, &sh.NodeMIME, &trashedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	sh.CreatedAt = time.Unix(createdUnix, 0).UTC()
	sh.UpdatedAt = time.Unix(updatedUnix, 0).UTC()
	if pwdHash.Valid && pwdHash.String != "" {
		sh.HasPassword = true
		sh.PasswordHash = &pwdHash.String
	}
	if expUnix.Valid {
		t := time.Unix(expUnix.Int64, 0).UTC()
		sh.ExpiresAt = &t
	}
	if revokedUnix.Valid {
		t := time.Unix(revokedUnix.Int64, 0).UTC()
		sh.RevokedAt = &t
	}

	if trashedAt.Valid {
		return &sh, ErrNodeTrashed
	}
	if sh.RevokedAt != nil {
		return &sh, ErrRevoked
	}
	if sh.ExpiresAt != nil && sh.ExpiresAt.Before(time.Now().UTC()) {
		return &sh, ErrExpired
	}

	return &sh, nil
}

func (s *Service) GetByNode(ctx context.Context, userID, nodeID string) (*Share, error) {
	var sh Share
	var pwdHash sql.NullString
	var expUnix sql.NullInt64
	var revokedUnix sql.NullInt64
	var createdUnix, updatedUnix int64

	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, node_id, slug, password_hash, expires_at, view_count, created_at, updated_at, revoked_at
		FROM shares
		WHERE user_id=? AND node_id=? AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1
	`, userID, nodeID).Scan(
		&sh.ID, &sh.UserID, &sh.NodeID, &sh.Slug, &pwdHash, &expUnix, &sh.ViewCount,
		&createdUnix, &updatedUnix, &revokedUnix,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	sh.CreatedAt = time.Unix(createdUnix, 0).UTC()
	sh.UpdatedAt = time.Unix(updatedUnix, 0).UTC()
	if pwdHash.Valid && pwdHash.String != "" {
		sh.HasPassword = true
		sh.PasswordHash = &pwdHash.String
	}
	if expUnix.Valid {
		t := time.Unix(expUnix.Int64, 0).UTC()
		sh.ExpiresAt = &t
	}
	if revokedUnix.Valid {
		t := time.Unix(revokedUnix.Int64, 0).UTC()
		sh.RevokedAt = &t
	}

	return &sh, nil
}

func (s *Service) ListByUser(ctx context.Context, userID string) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.user_id, s.node_id, s.slug, s.password_hash, s.expires_at, s.view_count,
		       s.created_at, s.updated_at, s.revoked_at,
		       n.name, n.kind, n.size_bytes, coalesce(n.mime_type, '')
		FROM shares s
		JOIN nodes n ON s.node_id = n.id
		WHERE s.user_id=? AND s.revoked_at IS NULL AND n.trashed_at IS NULL
		ORDER BY s.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Share
	for rows.Next() {
		var sh Share
		var pwdHash sql.NullString
		var expUnix, revokedUnix sql.NullInt64
		var createdUnix, updatedUnix int64

		if err := rows.Scan(
			&sh.ID, &sh.UserID, &sh.NodeID, &sh.Slug, &pwdHash, &expUnix, &sh.ViewCount,
			&createdUnix, &updatedUnix, &revokedUnix,
			&sh.NodeName, &sh.NodeKind, &sh.NodeSize, &sh.NodeMIME,
		); err != nil {
			return nil, err
		}

		sh.CreatedAt = time.Unix(createdUnix, 0).UTC()
		sh.UpdatedAt = time.Unix(updatedUnix, 0).UTC()
		if pwdHash.Valid && pwdHash.String != "" {
			sh.HasPassword = true
		}
		if expUnix.Valid {
			t := time.Unix(expUnix.Int64, 0).UTC()
			sh.ExpiresAt = &t
		}
		if revokedUnix.Valid {
			t := time.Unix(revokedUnix.Int64, 0).UTC()
			sh.RevokedAt = &t
		}
		result = append(result, sh)
	}
	return result, rows.Err()
}

func (s *Service) Revoke(ctx context.Context, userID, shareID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE shares SET revoked_at=unixepoch(), updated_at=unixepoch()
		WHERE id=? AND user_id=? AND revoked_at IS NULL
	`, shareID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) IncrementViews(ctx context.Context, shareID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE shares SET view_count = view_count + 1 WHERE id=?`, shareID)
	return err
}

func (s *Service) VerifyPassword(sh *Share, password string) bool {
	if !sh.HasPassword || sh.PasswordHash == nil {
		return true
	}
	return auth.VerifyPassword(*sh.PasswordHash, password)
}

// VerifyDescendant ensures childNodeID is rootNodeID or a descendant of rootNodeID within the user's active tree
func (s *Service) VerifyDescendant(ctx context.Context, rootNodeID, childNodeID string) (bool, error) {
	if rootNodeID == childNodeID {
		return true, nil
	}

	currentID := childNodeID
	for {
		var parentID sql.NullString
		var trashedAt sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT parent_id, trashed_at FROM nodes WHERE id=?`, currentID).Scan(&parentID, &trashedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if trashedAt.Valid {
			return false, nil
		}
		if !parentID.Valid {
			return false, nil
		}
		if parentID.String == rootNodeID {
			return true, nil
		}
		currentID = parentID.String
	}
}

// ListSharedChildren lists visible children for a shared folder
func (s *Service) ListSharedChildren(ctx context.Context, rootNodeID, folderID string) ([]files.Node, error) {
	ok, err := s.VerifyDescendant(ctx, rootNodeID, folderID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, coalesce(parent_id, ''), kind, name, coalesce(mime_type, ''), size_bytes, created_at, updated_at
		FROM nodes
		WHERE parent_id=? AND trashed_at IS NULL
		ORDER BY CASE WHEN kind='folder' THEN 0 ELSE 1 END, name COLLATE NOCASE ASC
	`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []files.Node
	for rows.Next() {
		var n files.Node
		var createdUnix, updatedUnix int64
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name, &n.MIMEType, &n.Size, &createdUnix, &updatedUnix); err != nil {
			return nil, err
		}
		n.CreatedAt = time.Unix(createdUnix, 0).UTC()
		n.UpdatedAt = time.Unix(updatedUnix, 0).UTC()
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// SharedBreadcrumbs calculates breadcrumb path relative to the shared root node
func (s *Service) SharedBreadcrumbs(ctx context.Context, rootNodeID, targetNodeID string) ([]files.Node, error) {
	ok, err := s.VerifyDescendant(ctx, rootNodeID, targetNodeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAccessDenied
	}

	var crumbs []files.Node
	curr := targetNodeID
	for {
		var n files.Node
		var parent sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT id, parent_id, kind, name FROM nodes WHERE id=?`, curr).Scan(&n.ID, &parent, &n.Kind, &n.Name)
		if err != nil {
			return nil, err
		}
		if parent.Valid {
			n.ParentID = parent.String
		}
		crumbs = append([]files.Node{n}, crumbs...)
		if curr == rootNodeID {
			break
		}
		if !parent.Valid {
			break
		}
		curr = parent.String
	}
	return crumbs, nil
}
