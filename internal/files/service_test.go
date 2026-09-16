package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlphaTechiess/alphadrive/internal/database"
)

func TestUploadUsesOpaqueStorageAndEnforcesOwnership(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := New(db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, user := range []string{"user-a", "user-b"} {
		if _, err := db.Exec(`INSERT INTO users(id,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?, 'x',0,1,1)`, user, user); err != nil {
			t.Fatal(err)
		}
		if err := service.EnsureRoot(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	n, err := service.Upload(ctx, "user-a", RootID("user-a"), "report.txt", "opaque-id", strings.NewReader("contents"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "report.txt" {
		t.Fatal("display name changed")
	}
	if _, err := os.Stat(filepath.Join(dir, "files", "objects", "opaque-id")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.OpenDownload(ctx, "user-b", n.ID); err != ErrNotFound {
		t.Fatalf("cross-user download: %v", err)
	}
	if _, err := service.Upload(ctx, "user-a", RootID("user-a"), "large.txt", "another-id", strings.NewReader("12345"), 4); err == nil {
		t.Fatal("oversized upload accepted")
	}
}

func TestBreadcrumbs(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := New(db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	user := "user-crumbs"
	_, _ = db.Exec(`INSERT INTO users(id,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?, 'x',0,1,1)`, user, user)
	_ = service.EnsureRoot(ctx, user)

	folder1, err := service.CreateFolder(ctx, user, RootID(user), "Docs", "f1")
	if err != nil {
		t.Fatal(err)
	}
	folder2, err := service.CreateFolder(ctx, user, folder1.ID, "Projects", "f2")
	if err != nil {
		t.Fatal(err)
	}

	crumbs, err := service.Breadcrumbs(ctx, user, folder2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(crumbs) != 3 {
		t.Fatalf("expected 3 crumbs, got %d", len(crumbs))
	}
	if crumbs[0].Name != "My drive" || crumbs[1].Name != "Docs" || crumbs[2].Name != "Projects" {
		t.Fatalf("unexpected crumbs: %+v", crumbs)
	}
}

func TestTrashRestoreAndPermanentDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service, err := New(db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	user := "user-trash"
	_, _ = db.Exec(`INSERT INTO users(id,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?, 'x',0,1,1)`, user, user)
	_ = service.EnsureRoot(ctx, user)

	folder, err := service.CreateFolder(ctx, user, RootID(user), "Archive", "f-arch")
	if err != nil {
		t.Fatal(err)
	}
	file, err := service.Upload(ctx, user, folder.ID, "data.csv", "csv-file-id", strings.NewReader("col1,col2\n1,2"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	// Verify storage usage calculation
	usage, err := service.StorageUsage(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if usage != file.Size {
		t.Fatalf("expected usage %d, got %d", file.Size, usage)
	}

	// Move folder to trash (should cascade to nested file)
	if err := service.Trash(ctx, user, []string{folder.ID}); err != nil {
		t.Fatal(err)
	}

	// Listing Root should not contain the trashed folder
	nodes, err := service.List(ctx, user, RootID(user))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 active nodes in root, got %d", len(nodes))
	}

	// Trashed file cannot be downloaded
	if _, _, err := service.OpenDownload(ctx, user, file.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for trashed file download, got %v", err)
	}

	// List trash should show folder
	trashList, err := service.ListTrash(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashList) != 1 || trashList[0].ID != folder.ID {
		t.Fatalf("expected folder in trash, got %+v", trashList)
	}

	// Restore folder (should cascade to nested file)
	if err := service.Restore(ctx, user, []string{folder.ID}); err != nil {
		t.Fatal(err)
	}

	// Verify file is downloadable again
	f, n, err := service.OpenDownload(ctx, user, file.ID)
	if err != nil {
		t.Fatalf("download after restore failed: %v", err)
	}
	f.Close()
	if n.Name != "data.csv" {
		t.Fatalf("wrong file name: %s", n.Name)
	}

	// Trash again and permanently delete
	if err := service.Trash(ctx, user, []string{folder.ID}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeletePermanently(ctx, user, []string{folder.ID}); err != nil {
		t.Fatal(err)
	}

	// Verify file object is removed from disk
	if _, err := os.Stat(filepath.Join(dir, "files", "objects", "csv-file-id")); !os.IsNotExist(err) {
		t.Fatalf("expected physical file to be removed from disk, stat err: %v", err)
	}

	// Verify node is removed from DB
	if _, err := service.Get(ctx, user, file.ID); err != ErrNotFound {
		t.Fatalf("expected node to be deleted from DB, got %v", err)
	}
}
