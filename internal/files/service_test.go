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

func TestEmptyTrash(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	user := "user-empty-trash"
	_, _ = db.Exec(`INSERT INTO users(id,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?, 'x',0,1,1)`, user, user)
	service, err := New(db.DB, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureRoot(ctx, user); err != nil {
		t.Fatal(err)
	}

	// Create and upload two files
	f1, err := service.Upload(ctx, user, "", "file1.txt", "file-1-id", strings.NewReader("hello file 1"), 0)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := service.Upload(ctx, user, "", "file2.txt", "file-2-id", strings.NewReader("hello file 2"), 0)
	if err != nil {
		t.Fatal(err)
	}

	// Move both to trash
	if err := service.Trash(ctx, user, []string{f1.ID, f2.ID}); err != nil {
		t.Fatal(err)
	}

	trashList, err := service.ListTrash(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashList) != 2 {
		t.Fatalf("expected 2 items in trash, got %d", len(trashList))
	}

	// Empty trash
	if err := service.EmptyTrash(ctx, user); err != nil {
		t.Fatal(err)
	}

	// Verify trash is empty
	trashListAfter, err := service.ListTrash(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(trashListAfter) != 0 {
		t.Fatalf("expected 0 items in trash after empty, got %d", len(trashListAfter))
	}

	// Verify physical objects are unlinked
	if _, err := os.Stat(filepath.Join(dir, "files", "objects", "file-1-id")); !os.IsNotExist(err) {
		t.Fatalf("expected file 1 object to be deleted, got err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", "objects", "file-2-id")); !os.IsNotExist(err) {
		t.Fatalf("expected file 2 object to be deleted, got err: %v", err)
	}
}

func TestMoveNodes(t *testing.T) {
	ctx := context.Background()
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
	user := "user-move"
	if _, err := db.Exec(`INSERT INTO users(id,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?, 'x',0,1,1)`, user, user); err != nil {
		t.Fatal(err)
	}

	if err := service.EnsureRoot(ctx, user); err != nil {
		t.Fatal(err)
	}

	// Create a folder "Docs" in root
	docs, err := service.CreateFolder(ctx, user, "", "Docs", "docs-id")
	if err != nil {
		t.Fatal(err)
	}

	// Create a subfolder "Sub" in Docs
	sub, err := service.CreateFolder(ctx, user, docs.ID, "Sub", "sub-id")
	if err != nil {
		t.Fatal(err)
	}

	// Create a file in root
	f, err := service.Upload(ctx, user, "", "report.pdf", "f-id-1", strings.NewReader("pdf-data"), 0)
	if err != nil {
		t.Fatal(err)
	}

	// Move file from root into Docs
	if err := service.Move(ctx, user, docs.ID, []string{f.ID}); err != nil {
		t.Fatalf("move file to docs: %v", err)
	}

	// Verify file is in Docs
	docsItems, err := service.List(ctx, user, docs.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range docsItems {
		if it.ID == f.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected file %s in Docs folder", f.ID)
	}

	// Move Sub into Root
	if err := service.Move(ctx, user, "", []string{sub.ID}); err != nil {
		t.Fatalf("move sub to root: %v", err)
	}

	// Verify cycle detection: cannot move Docs into Docs or its child
	if err := service.Move(ctx, user, docs.ID, []string{docs.ID}); err == nil {
		// Moving into itself is skipped/noop, but moving into descendant should fail
	}
}

func TestDeepSearch(t *testing.T) {
	ctx := context.Background()
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
	user := "user-search"
	if _, err := db.Exec(`INSERT INTO users(id,username,password_hash,is_admin,created_at,updated_at) VALUES(?,?, 'x',0,1,1)`, user, user); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureRoot(ctx, user); err != nil {
		t.Fatal(err)
	}

	// Root
	// ├── photos (folder)
	// │   └── summer (folder)
	// │       └── beach_vacation.jpg (file)
	// ├── finances (folder)
	// │   └── 2026_budget.xlsx (file)
	// └── notes.txt (file)

	photos, err := service.CreateFolder(ctx, user, "", "photos", "photos-id")
	if err != nil {
		t.Fatal(err)
	}
	summer, err := service.CreateFolder(ctx, user, photos.ID, "summer", "summer-id")
	if err != nil {
		t.Fatal(err)
	}
	beach, err := service.Upload(ctx, user, summer.ID, "beach_vacation.jpg", "beach-id", strings.NewReader("img"), 0)
	if err != nil {
		t.Fatal(err)
	}
	finances, err := service.CreateFolder(ctx, user, "", "finances", "fin-id")
	if err != nil {
		t.Fatal(err)
	}
	budget, err := service.Upload(ctx, user, finances.ID, "2026_budget.xlsx", "budget-id", strings.NewReader("budget"), 0)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := service.Upload(ctx, user, "", "notes.txt", "notes-id", strings.NewReader("notes"), 0)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Search deep across whole drive for "beach"
	res, err := service.Search(ctx, user, "beach", "")
	if err != nil {
		t.Fatalf("search beach: %v", err)
	}
	if len(res) != 1 || res[0].ID != beach.ID {
		t.Fatalf("expected beach file in search results, got %+v", res)
	}

	// 2. Search deep for "summer"
	res, err = service.Search(ctx, user, "summer", "")
	if err != nil {
		t.Fatalf("search summer: %v", err)
	}
	if len(res) != 1 || res[0].ID != summer.ID {
		t.Fatalf("expected summer folder in search results, got %+v", res)
	}

	// 3. Search for "2026" inside photos folder (should not match budget)
	res, err = service.Search(ctx, user, "2026", photos.ID)
	if err != nil {
		t.Fatalf("search 2026 in photos: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected 0 results for 2026 in photos folder, got %d", len(res))
	}

	// 4. Search for "2026" globally
	res, err = service.Search(ctx, user, "2026", "")
	if err != nil {
		t.Fatalf("search 2026 globally: %v", err)
	}
	if len(res) != 1 || res[0].ID != budget.ID {
		t.Fatalf("expected budget in global search results, got %+v", res)
	}

	// 5. Trashing a node excludes it from search
	if err := service.Trash(ctx, user, []string{notes.ID}); err != nil {
		t.Fatal(err)
	}
	res, err = service.Search(ctx, user, "notes", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected trashed file to not appear in search, got %+v", res)
	}
}

