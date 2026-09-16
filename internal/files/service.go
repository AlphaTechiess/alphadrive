package files

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlphaTechiess/alphadrive/internal/validation"
)

var ErrNotFound = errors.New("not found")

type Node struct {
	ID        string     `json:"id"`
	ParentID  string     `json:"parent_id"`
	Kind      string     `json:"kind"`
	Name      string     `json:"name"`
	MIMEType  string     `json:"mime_type,omitempty"`
	Size      int64      `json:"size_bytes"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `json:"trashed_at,omitempty"`
}

type Service struct {
	db            *sql.DB
	objects, temp string
}

func New(db *sql.DB, dataDir string) (*Service, error) {
	s := &Service{
		db:      db,
		objects: filepath.Join(dataDir, "files", "objects"),
		temp:    filepath.Join(dataDir, "files", "temp"),
	}
	for _, dir := range []string{s.objects, s.temp} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Service) EnsureRoot(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO nodes(id,user_id,parent_id,kind,name,created_at,updated_at) VALUES(?,?,NULL,'folder','Root',?,?)`, rootID(userID), userID, now(), now())
	return err
}

func RootID(userID string) string { return rootID(userID) }
func rootID(userID string) string { return "root_" + userID }

func (s *Service) List(ctx context.Context, userID, parentID string) ([]Node, error) {
	if parentID == "" {
		parentID = rootID(userID)
	}
	if !s.ownedFolder(ctx, userID, parentID) {
		return nil, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, coalesce(parent_id,''), kind, name, coalesce(mime_type,''), size_bytes, created_at, updated_at FROM nodes WHERE user_id=? AND parent_id=? AND trashed_at IS NULL ORDER BY kind DESC, name COLLATE NOCASE`, userID, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		var created, updated int64
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name, &n.MIMEType, &n.Size, &created, &updated); err != nil {
			return nil, err
		}
		n.CreatedAt = time.Unix(created, 0).UTC()
		n.UpdatedAt = time.Unix(updated, 0).UTC()
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, userID, id string) (Node, error) {
	if id == "" || id == rootID(userID) {
		id = rootID(userID)
	}
	var n Node
	var created, updated int64
	var trashed sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, coalesce(parent_id,''), kind, name, coalesce(mime_type,''), size_bytes, created_at, updated_at, trashed_at FROM nodes WHERE id=? AND user_id=?`, id, userID).Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name, &n.MIMEType, &n.Size, &created, &updated, &trashed)
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	if err != nil {
		return Node{}, err
	}
	if n.ID == rootID(userID) {
		n.Name = "My drive"
	}
	n.CreatedAt = time.Unix(created, 0).UTC()
	n.UpdatedAt = time.Unix(updated, 0).UTC()
	if trashed.Valid {
		t := time.Unix(trashed.Int64, 0).UTC()
		n.TrashedAt = &t
	}
	return n, nil
}

func (s *Service) Breadcrumbs(ctx context.Context, userID, nodeID string) ([]Node, error) {
	root := rootID(userID)
	if nodeID == "" || nodeID == root {
		return []Node{{ID: root, Name: "My drive", Kind: "folder"}}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE path(id, parent_id, kind, name, level) AS (
			SELECT id, coalesce(parent_id,''), kind, name, 0 FROM nodes WHERE id=? AND user_id=? AND trashed_at IS NULL
			UNION ALL
			SELECT n.id, coalesce(n.parent_id,''), n.kind, n.name, p.level + 1
			FROM nodes n JOIN path p ON n.id = p.parent_id WHERE n.user_id=? AND n.trashed_at IS NULL
		)
		SELECT id, parent_id, kind, name FROM path ORDER BY level DESC
	`, nodeID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var crumbs []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name); err != nil {
			return nil, err
		}
		if n.ID == root {
			n.Name = "My drive"
		}
		crumbs = append(crumbs, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(crumbs) == 0 {
		return nil, ErrNotFound
	}
	return crumbs, nil
}

func (s *Service) StorageUsage(ctx context.Context, userID string) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(size_bytes), 0) FROM nodes WHERE user_id=? AND kind='file'`, userID).Scan(&total)
	return total, err
}

func (s *Service) CreateFolder(ctx context.Context, userID, parentID, name, id string) (Node, error) {
	name, err := validation.Name(name)
	if err != nil {
		return Node{}, err
	}
	if parentID == "" {
		parentID = rootID(userID)
	}
	if !s.ownedFolder(ctx, userID, parentID) {
		return Node{}, ErrNotFound
	}
	t := now()
	_, err = s.db.ExecContext(ctx, `INSERT INTO nodes(id,user_id,parent_id,kind,name,created_at,updated_at) VALUES(?,?,?,'folder',?,?,?)`, id, userID, parentID, name, t, t)
	if err != nil {
		return Node{}, err
	}
	return Node{
		ID:        id,
		ParentID:  parentID,
		Kind:      "folder",
		Name:      name,
		CreatedAt: time.Unix(t, 0).UTC(),
		UpdatedAt: time.Unix(t, 0).UTC(),
	}, nil
}

func (s *Service) Upload(ctx context.Context, userID, parentID, filename, id string, src io.Reader, max int64) (Node, error) {
	filename, err := validation.Name(filename)
	if err != nil {
		return Node{}, err
	}
	if parentID == "" {
		parentID = rootID(userID)
	}
	if !s.ownedFolder(ctx, userID, parentID) {
		return Node{}, ErrNotFound
	}
	tmp, err := os.CreateTemp(s.temp, "upload-*")
	if err != nil {
		return Node{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	written, err := io.Copy(tmp, io.LimitReader(src, max+1))
	closeErr := tmp.Close()
	if err != nil {
		return Node{}, err
	}
	if closeErr != nil {
		return Node{}, closeErr
	}
	if written > max {
		return Node{}, fmt.Errorf("upload exceeds configured size limit")
	}
	storageKey := id
	final := filepath.Join(s.objects, storageKey)
	if err := os.Rename(tmpName, final); err != nil {
		return Node{}, err
	}
	t := now()
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if mt == "" {
		mt = "application/octet-stream"
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO nodes(id,user_id,parent_id,kind,name,storage_key,mime_type,size_bytes,created_at,updated_at) VALUES(?,?,?,'file',?,?,?,?,?,?)`, id, userID, parentID, filename, storageKey, mt, written, t, t)
	if err != nil {
		_ = os.Remove(final)
		return Node{}, err
	}
	return Node{
		ID:        id,
		ParentID:  parentID,
		Kind:      "file",
		Name:      filename,
		MIMEType:  mt,
		Size:      written,
		CreatedAt: time.Unix(t, 0).UTC(),
		UpdatedAt: time.Unix(t, 0).UTC(),
	}, nil
}

func (s *Service) OpenDownload(ctx context.Context, userID, id string) (*os.File, Node, error) {
	var n Node
	var key string
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT id, coalesce(parent_id,''), kind, name, storage_key, coalesce(mime_type,''), size_bytes, created_at, updated_at FROM nodes WHERE id=? AND user_id=? AND trashed_at IS NULL`, id, userID).Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name, &key, &n.MIMEType, &n.Size, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, Node{}, ErrNotFound
	}
	if err != nil {
		return nil, Node{}, err
	}
	if n.Kind != "file" {
		return nil, Node{}, ErrNotFound
	}
	n.CreatedAt = time.Unix(created, 0).UTC()
	n.UpdatedAt = time.Unix(updated, 0).UTC()
	if filepath.Base(key) != key {
		return nil, Node{}, fmt.Errorf("invalid storage key")
	}
	f, err := os.Open(filepath.Join(s.objects, key))
	if os.IsNotExist(err) {
		return nil, Node{}, ErrNotFound
	}
	return f, n, err
}

func (s *Service) ListTrash(ctx context.Context, userID string) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, coalesce(n.parent_id,''), n.kind, n.name, coalesce(n.mime_type,''), n.size_bytes, n.created_at, n.updated_at, n.trashed_at
		FROM nodes n
		LEFT JOIN nodes p ON n.parent_id = p.id
		WHERE n.user_id = ? AND n.trashed_at IS NOT NULL AND (p.trashed_at IS NULL OR n.parent_id IS NULL)
		ORDER BY n.trashed_at DESC, n.name COLLATE NOCASE
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		var created, updated int64
		var trashed sql.NullInt64
		if err := rows.Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name, &n.MIMEType, &n.Size, &created, &updated, &trashed); err != nil {
			return nil, err
		}
		n.CreatedAt = time.Unix(created, 0).UTC()
		n.UpdatedAt = time.Unix(updated, 0).UTC()
		if trashed.Valid {
			t := time.Unix(trashed.Int64, 0).UTC()
			n.TrashedAt = &t
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Service) Trash(ctx context.Context, userID string, nodeIDs []string) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	root := rootID(userID)
	t := now()
	for _, id := range nodeIDs {
		if id == root || id == "" {
			continue
		}
		_, err := s.db.ExecContext(ctx, `
			WITH RECURSIVE subnodes(id) AS (
				SELECT id FROM nodes WHERE id=? AND user_id=? AND id != ?
				UNION ALL
				SELECT n.id FROM nodes n JOIN subnodes s ON n.parent_id = s.id WHERE n.user_id=?
			)
			UPDATE nodes SET trashed_at=?, updated_at=? WHERE id IN (SELECT id FROM subnodes) AND trashed_at IS NULL
		`, id, userID, root, userID, t, t)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Restore(ctx context.Context, userID string, nodeIDs []string) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	root := rootID(userID)
	t := now()
	for _, id := range nodeIDs {
		if id == root || id == "" {
			continue
		}
		var parentID, name, kind string
		err := s.db.QueryRowContext(ctx, `SELECT coalesce(parent_id,''), name, kind FROM nodes WHERE id=? AND user_id=? AND trashed_at IS NOT NULL`, id, userID).Scan(&parentID, &name, &kind)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}

		if parentID != "" && parentID != root {
			var activeParent int
			_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE id=? AND user_id=? AND kind='folder' AND trashed_at IS NULL`, parentID, userID).Scan(&activeParent)
			if activeParent == 0 {
				parentID = root
				_, _ = s.db.ExecContext(ctx, `UPDATE nodes SET parent_id=? WHERE id=? AND user_id=?`, parentID, id, userID)
			}
		} else if parentID == "" {
			parentID = root
			_, _ = s.db.ExecContext(ctx, `UPDATE nodes SET parent_id=? WHERE id=? AND user_id=?`, parentID, id, userID)
		}

		var conflict int
		_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE user_id=? AND parent_id=? AND name=? AND trashed_at IS NULL AND id!=?`, userID, parentID, name, id).Scan(&conflict)
		if conflict > 0 {
			newName := fmt.Sprintf("%s (restored %d)", name, time.Now().Unix()%10000)
			_, _ = s.db.ExecContext(ctx, `UPDATE nodes SET name=? WHERE id=? AND user_id=?`, newName, id, userID)
		}

		_, err = s.db.ExecContext(ctx, `
			WITH RECURSIVE subnodes(id) AS (
				SELECT id FROM nodes WHERE id=? AND user_id=?
				UNION ALL
				SELECT n.id FROM nodes n JOIN subnodes s ON n.parent_id = s.id WHERE n.user_id=?
			)
			UPDATE nodes SET trashed_at=NULL, updated_at=? WHERE id IN (SELECT id FROM subnodes)
		`, id, userID, userID, t)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeletePermanently(ctx context.Context, userID string, nodeIDs []string) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	root := rootID(userID)
	for _, id := range nodeIDs {
		if id == root || id == "" {
			continue
		}
		rows, err := s.db.QueryContext(ctx, `
			WITH RECURSIVE subnodes(id, kind, storage_key) AS (
				SELECT id, kind, coalesce(storage_key,'') FROM nodes WHERE id=? AND user_id=? AND id != ?
				UNION ALL
				SELECT n.id, n.kind, coalesce(n.storage_key,'') FROM nodes n JOIN subnodes s ON n.parent_id = s.id WHERE n.user_id=?
			)
			SELECT id, kind, storage_key FROM subnodes
		`, id, userID, root, userID)
		if err != nil {
			return err
		}

		type toDelete struct {
			id, kind, key string
		}
		var items []toDelete
		for rows.Next() {
			var td toDelete
			if err := rows.Scan(&td.id, &td.kind, &td.key); err != nil {
				rows.Close()
				return err
			}
			items = append(items, td)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, it := range items {
			if it.kind == "file" && it.key != "" && filepath.Base(it.key) == it.key {
				_ = os.Remove(filepath.Join(s.objects, it.key))
			}
		}

		if len(items) > 0 {
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			for _, it := range items {
				_, _ = tx.ExecContext(ctx, `DELETE FROM nodes WHERE id=? AND user_id=?`, it.id, userID)
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) WriteZip(ctx context.Context, userID string, nodeIDs []string, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, id := range nodeIDs {
		var n Node
		var key string
		var created, updated int64
		err := s.db.QueryRowContext(ctx, `SELECT id, coalesce(parent_id,''), kind, name, coalesce(storage_key,''), coalesce(mime_type,''), size_bytes, created_at, updated_at FROM nodes WHERE id=? AND user_id=? AND trashed_at IS NULL`, id, userID).Scan(&n.ID, &n.ParentID, &n.Kind, &n.Name, &key, &n.MIMEType, &n.Size, &created, &updated)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		n.CreatedAt = time.Unix(created, 0).UTC()
		n.UpdatedAt = time.Unix(updated, 0).UTC()

		if n.Kind == "file" {
			if err := s.addFileToZip(zw, n.Name, key, n.UpdatedAt); err != nil {
				return err
			}
		} else if n.Kind == "folder" {
			if err := s.addFolderToZip(ctx, userID, zw, n.ID, n.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) addFolderToZip(ctx context.Context, userID string, zw *zip.Writer, folderID, prefix string) error {
	_, err := zw.CreateHeader(&zip.FileHeader{
		Name:     strings.TrimSuffix(prefix, "/") + "/",
		Method:   zip.Deflate,
		Modified: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, name, coalesce(storage_key,''), updated_at FROM nodes WHERE user_id=? AND parent_id=? AND trashed_at IS NULL ORDER BY kind DESC, name COLLATE NOCASE`, userID, folderID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type child struct {
		id, kind, name, key string
		updated             int64
	}
	var children []child
	for rows.Next() {
		var c child
		if err := rows.Scan(&c.id, &c.kind, &c.name, &c.key, &c.updated); err != nil {
			return err
		}
		children = append(children, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, c := range children {
		itemPath := prefix + "/" + c.name
		if c.kind == "file" {
			if err := s.addFileToZip(zw, itemPath, c.key, time.Unix(c.updated, 0).UTC()); err != nil {
				return err
			}
		} else if c.kind == "folder" {
			if err := s.addFolderToZip(ctx, userID, zw, c.id, itemPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) addFileToZip(zw *zip.Writer, zipPath, storageKey string, modified time.Time) error {
	if filepath.Base(storageKey) != storageKey {
		return fmt.Errorf("invalid storage key")
	}
	f, err := os.Open(filepath.Join(s.objects, storageKey))
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate
	if !modified.IsZero() {
		header.Modified = modified
	}
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func (s *Service) ownedFolder(ctx context.Context, userID, id string) bool {
	var n int
	return s.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE id=? AND user_id=? AND kind='folder' AND trashed_at IS NULL`, id, userID).Scan(&n) == nil && n == 1
}

func now() int64 { return time.Now().UTC().Unix() }
