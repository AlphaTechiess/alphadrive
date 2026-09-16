package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlphaTechiess/alphadrive/internal/database"
	"github.com/AlphaTechiess/alphadrive/internal/files"
)

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	db, err := database.Open(srcDir)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	fs, err := files.New(db.DB, srcDir)
	if err != nil {
		t.Fatalf("new files: %v", err)
	}

	// Insert test data
	userID := "user_test_backup"
	_, err = db.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at, updated_at) VALUES(?, 'backupuser', 'hash', 0, unixepoch(), unixepoch())`, userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := fs.EnsureRoot(context.Background(), userID); err != nil {
		t.Fatalf("ensure root: %v", err)
	}

	// Create test physical file
	testContent := []byte("This is a backup test file object.")
	objDir := filepath.Join(srcDir, "files", "objects")
	if err := os.MkdirAll(objDir, 0700); err != nil {
		t.Fatalf("mkdir objects: %v", err)
	}
	if err := os.WriteFile(filepath.Join(objDir, "test-key-123"), testContent, 0600); err != nil {
		t.Fatalf("write object: %v", err)
	}

	// Create backup
	archivePath := filepath.Join(t.TempDir(), "backup-test.tar.gz")
	if err := Create(context.Background(), db.DB, srcDir, archivePath); err != nil {
		t.Fatalf("Create backup failed: %v", err)
	}

	db.Close()

	// Verify archive exists
	stat, err := os.Stat(archivePath)
	if err != nil || stat.Size() == 0 {
		t.Fatalf("invalid archive file created: %v", err)
	}

	// Restore into fresh directory
	restoreDir := t.TempDir()
	info, err := Restore(context.Background(), archivePath, restoreDir)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if info.ObjectCount != 1 {
		t.Fatalf("expected 1 object in metadata, got %d", info.ObjectCount)
	}

	// Verify restored physical object
	restoredObj := filepath.Join(restoreDir, "files", "objects", "test-key-123")
	data, err := os.ReadFile(restoredObj)
	if err != nil {
		t.Fatalf("read restored object: %v", err)
	}
	if string(data) != string(testContent) {
		t.Fatalf("restored content mismatch: got %q, want %q", string(data), string(testContent))
	}

	// Verify restored database opens cleanly
	restoredDB, err := database.Open(restoreDir)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restoredDB.Close()

	var rootCount int
	err = restoredDB.QueryRow("SELECT count(*) FROM nodes WHERE id=?", files.RootID(userID)).Scan(&rootCount)
	if err != nil || rootCount != 1 {
		t.Fatalf("expected 1 root node in restored database, got %d (err: %v)", rootCount, err)
	}
}
