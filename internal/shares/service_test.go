package shares

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/AlphaTechiess/alphadrive/internal/database"
	"github.com/AlphaTechiess/alphadrive/internal/files"
)

func setupTestDB(t *testing.T) (*database.DB, *files.Service, *Service, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	fs, err := files.New(db.DB, dir)
	if err != nil {
		t.Fatalf("new files: %v", err)
	}
	ss := New(db.DB)

	userID := "user_1"
	_, err = db.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at, updated_at) VALUES(?, 'testuser', 'hash', 0, unixepoch(), unixepoch())`, userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := fs.EnsureRoot(context.Background(), userID); err != nil {
		t.Fatalf("ensure root: %v", err)
	}
	return db, fs, ss, userID
}

func TestShareCreationAndSlugValidation(t *testing.T) {
	ctx := context.Background()
	db, fs, ss, userID := setupTestDB(t)
	defer db.Close()

	rootID := files.RootID(userID)
	folder, err := fs.CreateFolder(ctx, userID, rootID, "PublicDocs", "node_folder1")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}

	// 1. Auto-generated slug
	sh1, err := ss.Create(ctx, userID, folder.ID, "", "", nil)
	if err != nil {
		t.Fatalf("create share auto slug: %v", err)
	}
	if sh1.Slug == "" || len(sh1.Slug) < 3 {
		t.Errorf("expected generated slug, got %s", sh1.Slug)
	}
	if err := ValidateSlug(sh1.Slug); err != nil {
		t.Errorf("generated slug failed validation: %v", err)
	}

	// 2. Custom valid slug
	sh2, err := ss.Create(ctx, userID, folder.ID, "my-custom-docs", "", nil)
	if err != nil {
		t.Fatalf("create share custom slug: %v", err)
	}
	if sh2.Slug != "my-custom-docs" {
		t.Errorf("expected 'my-custom-docs', got %s", sh2.Slug)
	}

	// 3. Duplicate slug rejection
	_, err = ss.Create(ctx, userID, folder.ID, "my-custom-docs", "", nil)
	if err != ErrSlugTaken {
		t.Errorf("expected ErrSlugTaken, got %v", err)
	}

	// 4. Invalid slug formats (including underscores and uppercase)
	invalidSlugs := []string{"a", "ab", "invalid_with_underscore", "UPPERCASE", "with spaces", "bad/slash", "api", "static", "login", "healthz"}
	for _, s := range invalidSlugs {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("expected ValidateSlug error for '%s', got nil", s)
		}
		_, err := ss.Create(ctx, userID, folder.ID, s, "", nil)
		if err == nil {
			t.Errorf("expected Create error for invalid slug '%s', got nil", s)
		}
	}
}

func TestShareExpiryParsing(t *testing.T) {
	// Valid expiries
	for _, exp := range []string{"1h", "24h", "7d", "30d"} {
		parsed, err := ParseExpiry(exp)
		if err != nil {
			t.Errorf("expected valid ParseExpiry for %s, got err: %v", exp, err)
		}
		if parsed == nil || !parsed.After(time.Now().UTC()) {
			t.Errorf("expected future time for %s, got %v", exp, parsed)
		}
	}

	// Never / empty
	for _, exp := range []string{"", "never", "none", "  "} {
		parsed, err := ParseExpiry(exp)
		if err != nil || parsed != nil {
			t.Errorf("expected nil for %q, got parsed=%v, err=%v", exp, parsed, err)
		}
	}

	// Invalid expiries
	for _, exp := range []string{"invalid", "10m", "2years", "yesterday", "-1h"} {
		_, err := ParseExpiry(exp)
		if err == nil {
			t.Errorf("expected error for invalid expiry %q, got nil", exp)
		}
	}
}

func TestSharePasswordAndGrants(t *testing.T) {
	ctx := context.Background()
	db, fs, ss, userID := setupTestDB(t)
	defer db.Close()

	rootID := files.RootID(userID)
	file, err := fs.Upload(ctx, userID, rootID, "secret.txt", "node_sec", strings.NewReader("secret content"), 1024)
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}

	// Reject short share password (<8 chars)
	_, err = ss.Create(ctx, userID, file.ID, "short-pwd", "short", nil)
	if err == nil {
		t.Fatal("expected error for short share password (<8 chars), got nil")
	}

	// Create share with 12+ char password and future expiration
	future := time.Now().Add(24 * time.Hour).UTC()
	sh, err := ss.Create(ctx, userID, file.ID, "secret-share", "SuperSecretPass123", &future)
	if err != nil {
		t.Fatalf("create share: %v", err)
	}

	if !sh.HasPassword {
		t.Errorf("expected HasPassword=true")
	}

	// Fetch share
	fetched, err := ss.GetBySlug(ctx, "secret-share")
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if !ss.VerifyPassword(fetched, "SuperSecretPass123") {
		t.Errorf("password verification failed with correct password")
	}
	if ss.VerifyPassword(fetched, "WrongPass123456") {
		t.Errorf("password verification succeeded with wrong password")
	}

	// 1. Create grant for share
	grantToken1, err := ss.CreateGrant(ctx, sh.ID, 24*time.Hour, sh.ExpiresAt)
	if err != nil {
		t.Fatalf("create grant 1: %v", err)
	}
	if len(grantToken1) < 32 {
		t.Fatalf("expected grant token length >= 32, got %d", len(grantToken1))
	}

	// 2. Create second grant - must be different (randomness test)
	grantToken2, err := ss.CreateGrant(ctx, sh.ID, 24*time.Hour, sh.ExpiresAt)
	if err != nil {
		t.Fatalf("create grant 2: %v", err)
	}
	if grantToken1 == grantToken2 {
		t.Fatal("expected two generated grant tokens to be distinct")
	}

	// 3. Verify that DB stores only SHA-256 hash, not raw token
	tokenHash := sha256.Sum256([]byte(grantToken1))
	var storedHash []byte
	err = db.QueryRowContext(ctx, "SELECT token_hash FROM share_grants WHERE share_id=? AND token_hash=?", sh.ID, tokenHash[:]).Scan(&storedHash)
	if err != nil {
		t.Fatalf("failed to find token hash in database: %v", err)
	}

	// 4. Verify grant validity
	ok, err := ss.VerifyGrant(ctx, sh.ID, grantToken1)
	if err != nil || !ok {
		t.Fatalf("expected grant 1 to be valid, ok=%v, err=%v", ok, err)
	}

	// Invalid token fails
	badOk, _ := ss.VerifyGrant(ctx, sh.ID, "completely-fake-token")
	if badOk {
		t.Fatal("fake token was accepted as valid grant")
	}

	// 5. Revoke share -> grants must be invalidated
	if err := ss.Revoke(ctx, userID, sh.ID); err != nil {
		t.Fatalf("revoke share: %v", err)
	}

	revokedOk, _ := ss.VerifyGrant(ctx, sh.ID, grantToken1)
	if revokedOk {
		t.Fatal("grant is still valid after share was revoked")
	}
}

func TestSharedFolderContainmentAndBreadcrumbs(t *testing.T) {
	ctx := context.Background()
	db, fs, ss, userID := setupTestDB(t)
	defer db.Close()

	rootID := files.RootID(userID)
	parentFolder, err := fs.CreateFolder(ctx, userID, rootID, "SharedProject", "node_parent")
	if err != nil {
		t.Fatalf("create parent folder: %v", err)
	}

	subFolder, err := fs.CreateFolder(ctx, userID, parentFolder.ID, "SubModule", "node_sub")
	if err != nil {
		t.Fatalf("create sub folder: %v", err)
	}

	subFile, err := fs.Upload(ctx, userID, subFolder.ID, "code.go", "node_code", strings.NewReader("package main"), 1024)
	if err != nil {
		t.Fatalf("upload file: %v", err)
	}

	outsideFolder, err := fs.CreateFolder(ctx, userID, rootID, "PrivateOutside", "node_outside")
	if err != nil {
		t.Fatalf("create outside folder: %v", err)
	}

	// Create share on parentFolder
	sh, err := ss.Create(ctx, userID, parentFolder.ID, "shared-project", "", nil)
	if err != nil {
		t.Fatalf("create share: %v", err)
	}

	// Verify containment
	ok, err := ss.VerifyDescendant(ctx, sh.NodeID, subFolder.ID)
	if err != nil || !ok {
		t.Errorf("expected subFolder to be descendant, ok=%v, err=%v", ok, err)
	}

	ok, err = ss.VerifyDescendant(ctx, sh.NodeID, subFile.ID)
	if err != nil || !ok {
		t.Errorf("expected subFile to be descendant, ok=%v, err=%v", ok, err)
	}

	ok, err = ss.VerifyDescendant(ctx, sh.NodeID, outsideFolder.ID)
	if err != nil || ok {
		t.Errorf("expected outsideFolder NOT to be descendant, ok=%v, err=%v", ok, err)
	}

	// Shared breadcrumbs
	crumbs, err := ss.SharedBreadcrumbs(ctx, sh.NodeID, subFolder.ID)
	if err != nil {
		t.Fatalf("shared breadcrumbs: %v", err)
	}
	if len(crumbs) != 2 {
		t.Fatalf("expected 2 crumbs, got %d", len(crumbs))
	}
	if crumbs[0].ID != parentFolder.ID || crumbs[1].ID != subFolder.ID {
		t.Errorf("unexpected crumbs: %+v", crumbs)
	}

	// Children listing
	children, err := ss.ListSharedChildren(ctx, sh.NodeID, parentFolder.ID)
	if err != nil {
		t.Fatalf("list shared children: %v", err)
	}
	if len(children) != 1 || children[0].ID != subFolder.ID {
		t.Errorf("unexpected children: %+v", children)
	}
}
