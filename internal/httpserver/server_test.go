package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlphaTechiess/alphadrive/internal/auth"
	"github.com/AlphaTechiess/alphadrive/internal/config"
	"github.com/AlphaTechiess/alphadrive/internal/database"
	"github.com/AlphaTechiess/alphadrive/internal/doctor"
	"github.com/AlphaTechiess/alphadrive/internal/files"
	"github.com/AlphaTechiess/alphadrive/internal/shares"
)

type testRig struct {
	server *httptest.Server
	db     *database.DB
	files  *files.Service
	shares *shares.Service
	cfg    config.Config
	dir    string
}

func setupTestRig(t *testing.T) *testRig {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(dir)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	fileService, err := files.New(db.DB, dir)
	if err != nil {
		t.Fatalf("new file service: %v", err)
	}
	shareService := shares.New(db.DB)

	cfg := config.Default()
	cfg.DataDir = dir
	cfg.SecureCookies = false
	cfg.MaxUploadBytes = 10 * 1024 * 1024 // 10MB
	cfg.StorageQuotaBytes = 1024 * 1024   // 1MB

	srv := New(cfg, db.DB, fileService, shareService)
	ts := httptest.NewServer(srv.Handler())

	t.Cleanup(func() {
		ts.Close()
		db.Close()
	})

	return &testRig{
		server: ts,
		db:     db,
		files:  fileService,
		shares: shareService,
		cfg:    cfg,
		dir:    dir,
	}
}

type authenticatedClient struct {
	client   *http.Client
	csrf     string
	username string
	userID   string
	rootID   string
}

func (r *testRig) createUser(t *testing.T, username, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	s := New(r.cfg, r.db.DB, r.files, r.shares)
	if err := s.CreateUser(context.Background(), username, hash, false); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func (r *testRig) login(t *testing.T, username, password string) *authenticatedClient {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	// 1. GET /login to retrieve login CSRF cookie
	resp, err := client.Get(r.server.URL + "/login")
	if err != nil {
		t.Fatalf("get login: %v", err)
	}
	defer resp.Body.Close()

	var loginCSRF string
	for _, c := range client.Jar.Cookies(resp.Request.URL) {
		if c.Name == "alphadrive_login_csrf" {
			loginCSRF = c.Value
			break
		}
	}
	if loginCSRF == "" {
		t.Fatalf("login CSRF cookie missing")
	}

	// 2. POST /login with credentials
	form := url.Values{
		"username": {username},
		"password": {password},
		"csrf":     {loginCSRF},
	}
	postResp, err := client.PostForm(r.server.URL+"/login", form)
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 after redirect, got %d", postResp.StatusCode)
	}

	// 3. GET /api/me to retrieve CSRF token and principal details
	meReq, _ := http.NewRequest("GET", r.server.URL+"/api/me", nil)
	meResp, err := client.Do(meReq)
	if err != nil {
		t.Fatalf("get me: %v", err)
	}
	defer meResp.Body.Close()

	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("expected /api/me status 200, got %d", meResp.StatusCode)
	}

	var meData struct {
		Username   string `json:"username"`
		RootID     string `json:"root_id"`
		UsedBytes  int64  `json:"used_bytes"`
		QuotaBytes int64  `json:"quota_bytes"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&meData); err != nil {
		t.Fatalf("decode /api/me: %v", err)
	}

	// Fetch CSRF token for principal
	var sessionTokenHash []byte
	for _, c := range client.Jar.Cookies(meReq.URL) {
		if c.Name == sessionCookie {
			_, h, _ := auth.TokenFromRaw(c.Value)
			sessionTokenHash = h
			break
		}
	}
	var csrfSecret []byte
	var userID string
	err = r.db.QueryRow(`SELECT user_id, csrf_secret FROM sessions WHERE token_hash=?`, sessionTokenHash).Scan(&userID, &csrfSecret)
	if err != nil {
		t.Fatalf("query session secret: %v", err)
	}

	csrfToken := auth.CSRF(csrfSecret)

	return &authenticatedClient{
		client:   client,
		csrf:     csrfToken,
		username: username,
		userID:   userID,
		rootID:   meData.RootID,
	}
}

func (ac *authenticatedClient) doJSON(t *testing.T, method, urlStr string, body any, target any) int {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-CSRF-Token", ac.csrf)
	resp, err := ac.client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if target != nil && resp.StatusCode < 400 {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("decode target: %v", err)
		}
	}
	return resp.StatusCode
}

func (ac *authenticatedClient) uploadFile(t *testing.T, serverURL, parentID, filename, content string) (files.Node, int) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("parent_id", parentID); err != nil {
		t.Fatalf("write field: %v", err)
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatalf("write content: %v", err)
	}
	mw.Close()

	req, err := http.NewRequest("POST", serverURL+"/api/uploads", &buf)
	if err != nil {
		t.Fatalf("new upload req: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", ac.csrf)

	resp, err := ac.client.Do(req)
	if err != nil {
		t.Fatalf("do upload: %v", err)
	}
	defer resp.Body.Close()

	var node files.Node
	if resp.StatusCode == http.StatusCreated {
		if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
			t.Fatalf("decode upload node: %v", err)
		}
	}
	return node, resp.StatusCode
}

func TestAuthAndSessionFlow(t *testing.T) {
	rig := setupTestRig(t)
	username := "testuser"
	password := "securePassphrase123"
	rig.createUser(t, username, password)

	// Bad password
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginResp, _ := client.Get(rig.server.URL + "/login")
	var loginCSRF string
	for _, c := range client.Jar.Cookies(loginResp.Request.URL) {
		if c.Name == "alphadrive_login_csrf" {
			loginCSRF = c.Value
		}
	}
	badForm := url.Values{"username": {username}, "password": {"wrongPassword123"}, "csrf": {loginCSRF}}
	resp, err := client.PostForm(rig.server.URL+"/login", badForm)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad password, got %d", resp.StatusCode)
	}

	// Valid login
	ac := rig.login(t, username, password)
	if ac.username != username {
		t.Fatalf("expected %s, got %s", username, ac.username)
	}

	// /api/me verification
	var me struct {
		Username                  string `json:"username"`
		RootID                    string `json:"root_id"`
		UsedBytes                 int64  `json:"used_bytes"`
		AlphadriveUsedBytes       int64  `json:"alphadrive_used_bytes"`
		StorageDetectionAvailable bool   `json:"storage_detection_available"`
		FilesystemTotalBytes      uint64 `json:"filesystem_total_bytes"`
		ServerTotal               uint64 `json:"server_total"`
	}
	status := ac.doJSON(t, "GET", rig.server.URL+"/api/me", nil, &me)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if me.Username != username || me.UsedBytes != 0 || me.AlphadriveUsedBytes != 0 {
		t.Fatalf("unexpected me response: %+v", me)
	}
}

func TestFolderHierarchyAndBreadcrumbs(t *testing.T) {
	rig := setupTestRig(t)
	user := "navuser"
	pass := "superSecretPass123"
	rig.createUser(t, user, pass)
	ac := rig.login(t, user, pass)

	// Create root child folder: "Documents"
	var docFolder files.Node
	status := ac.doJSON(t, "POST", rig.server.URL+"/api/folders", map[string]string{
		"parent_id": ac.rootID,
		"name":      "Documents",
	}, &docFolder)
	if status != http.StatusCreated || docFolder.Name != "Documents" {
		t.Fatalf("create folder status %d, folder %+v", status, docFolder)
	}

	// Create nested folder: "Documents/Work"
	var workFolder files.Node
	status = ac.doJSON(t, "POST", rig.server.URL+"/api/folders", map[string]string{
		"parent_id": docFolder.ID,
		"name":      "Work",
	}, &workFolder)
	if status != http.StatusCreated || workFolder.Name != "Work" {
		t.Fatalf("create nested folder status %d, folder %+v", status, workFolder)
	}

	// List "Documents/Work" and verify breadcrumbs
	var listResp struct {
		Nodes       []files.Node `json:"nodes"`
		Breadcrumbs []files.Node `json:"breadcrumbs"`
		Current     files.Node   `json:"current"`
	}
	status = ac.doJSON(t, "GET", rig.server.URL+"/api/nodes?parent_id="+workFolder.ID, nil, &listResp)
	if status != http.StatusOK {
		t.Fatalf("list nodes status %d", status)
	}
	if len(listResp.Breadcrumbs) != 3 {
		t.Fatalf("expected 3 breadcrumbs, got %d", len(listResp.Breadcrumbs))
	}
	if listResp.Breadcrumbs[0].Name != "My drive" || listResp.Breadcrumbs[1].Name != "Documents" || listResp.Breadcrumbs[2].Name != "Work" {
		t.Fatalf("unexpected breadcrumbs: %+v", listResp.Breadcrumbs)
	}
	if listResp.Current.Name != "Work" {
		t.Fatalf("expected current node Work, got %s", listResp.Current.Name)
	}
}

func TestUploadDownloadAndZip(t *testing.T) {
	rig := setupTestRig(t)
	user := "uploaduser"
	pass := "superSecretPass123"
	rig.createUser(t, user, pass)
	ac := rig.login(t, user, pass)

	// Upload a file
	content := "Hello AlphaDrive Integration Test!"
	fileNode, status := ac.uploadFile(t, rig.server.URL, ac.rootID, "greeting.txt", content)
	if status != http.StatusCreated {
		t.Fatalf("upload failed with status %d", status)
	}
	if fileNode.Name != "greeting.txt" || fileNode.Size != int64(len(content)) {
		t.Fatalf("unexpected file node: %+v", fileNode)
	}

	// Single file download
	dlResp, err := ac.client.Get(rig.server.URL + "/api/files/" + fileNode.ID + "/download")
	if err != nil {
		t.Fatalf("download file: %v", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("download status %d", dlResp.StatusCode)
	}
	dlBytes, _ := io.ReadAll(dlResp.Body)
	if string(dlBytes) != content {
		t.Fatalf("download content mismatch: got %q, want %q", string(dlBytes), content)
	}
	if dlResp.Header.Get("Content-Disposition") != `attachment; filename="greeting.txt"` {
		t.Fatalf("unexpected content disposition: %s", dlResp.Header.Get("Content-Disposition"))
	}

	// File info endpoint
	var infoNode files.Node
	status = ac.doJSON(t, "GET", rig.server.URL+"/api/nodes/"+fileNode.ID, nil, &infoNode)
	if status != http.StatusOK || infoNode.ID != fileNode.ID {
		t.Fatalf("get node info status %d, node %+v", status, infoNode)
	}

	// Multi-item / Folder ZIP download
	var zipReqBody = map[string][]string{"ids": {fileNode.ID}}
	jsonBytes, _ := json.Marshal(zipReqBody)
	req, _ := http.NewRequest("POST", rig.server.URL+"/api/nodes/download", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", ac.csrf)
	zipResp, err := ac.client.Do(req)
	if err != nil {
		t.Fatalf("zip download: %v", err)
	}
	defer zipResp.Body.Close()
	if zipResp.StatusCode != http.StatusOK {
		t.Fatalf("zip download status %d", zipResp.StatusCode)
	}
	zipData, err := io.ReadAll(zipResp.Body)
	if err != nil {
		t.Fatalf("read zip data: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("open zip reader: %v", err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "greeting.txt" {
		t.Fatalf("unexpected zip contents: %v", zr.File)
	}
	zf, _ := zr.File[0].Open()
	unzipped, _ := io.ReadAll(zf)
	zf.Close()
	if string(unzipped) != content {
		t.Fatalf("unzipped content mismatch: got %q, want %q", string(unzipped), content)
	}
}

func TestTrashRestoreAndPermanentDeleteLifecycle(t *testing.T) {
	rig := setupTestRig(t)
	user := "trashuser"
	pass := "superSecretPass123"
	rig.createUser(t, user, pass)
	ac := rig.login(t, user, pass)

	// Create folder & upload file inside folder
	var folder files.Node
	ac.doJSON(t, "POST", rig.server.URL+"/api/folders", map[string]string{"parent_id": ac.rootID, "name": "Projects"}, &folder)
	fileNode, _ := ac.uploadFile(t, rig.server.URL, folder.ID, "spec.pdf", "PDF file data")

	// 1. Move folder to trash
	var successResp map[string]bool
	status := ac.doJSON(t, "POST", rig.server.URL+"/api/nodes/trash", map[string][]string{"ids": {folder.ID}}, &successResp)
	if status != http.StatusOK || !successResp["success"] {
		t.Fatalf("trash status %d, resp %+v", status, successResp)
	}

	// 2. Trashed file cannot be downloaded (404)
	dlResp, _ := ac.client.Get(rig.server.URL + "/api/files/" + fileNode.ID + "/download")
	dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for trashed file download, got %d", dlResp.StatusCode)
	}

	// 3. List trash shows the folder
	var trashResp struct {
		Nodes []files.Node `json:"nodes"`
	}
	status = ac.doJSON(t, "GET", rig.server.URL+"/api/trash", nil, &trashResp)
	if status != http.StatusOK || len(trashResp.Nodes) != 1 || trashResp.Nodes[0].ID != folder.ID {
		t.Fatalf("list trash failed: status %d, nodes %+v", status, trashResp.Nodes)
	}

	// 4. Restore folder from trash
	status = ac.doJSON(t, "POST", rig.server.URL+"/api/nodes/restore", map[string][]string{"ids": {folder.ID}}, &successResp)
	if status != http.StatusOK || !successResp["success"] {
		t.Fatalf("restore status %d", status)
	}

	// 5. File is downloadable again after restore
	dlResp2, err := ac.client.Get(rig.server.URL + "/api/files/" + fileNode.ID + "/download")
	if err != nil || dlResp2.StatusCode != http.StatusOK {
		t.Fatalf("download after restore failed: status %d, err %v", dlResp2.StatusCode, err)
	}
	dlResp2.Body.Close()

	// 6. Move to trash and permanently delete
	ac.doJSON(t, "POST", rig.server.URL+"/api/nodes/trash", map[string][]string{"ids": {folder.ID}}, &successResp)
	status = ac.doJSON(t, "DELETE", rig.server.URL+"/api/nodes", map[string][]string{"ids": {folder.ID}}, &successResp)
	if status != http.StatusOK || !successResp["success"] {
		t.Fatalf("permanent delete status %d", status)
	}

	// 7. Verify file object is gone from disk
	if _, err := os.Stat(filepath.Join(rig.dir, "files", "objects", fileNode.ID)); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted from disk, stat err: %v", err)
	}
}

func TestAuthorizationAndCSRFFailures(t *testing.T) {
	rig := setupTestRig(t)
	userA := "user_alice"
	passA := "superSecretPass123"
	userB := "user_bob"
	passB := "superSecretPass123"

	rig.createUser(t, userA, passA)
	rig.createUser(t, userB, passB)

	acA := rig.login(t, userA, passA)
	acB := rig.login(t, userB, passB)

	// User A uploads a secret file
	fileA, _ := acA.uploadFile(t, rig.server.URL, acA.rootID, "alice_secret.txt", "Top secret")

	// 1. Unauthenticated request rejected (401)
	unauthResp, _ := http.Get(rig.server.URL + "/api/nodes")
	unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", unauthResp.StatusCode)
	}

	// 2. Missing/Invalid CSRF rejected (403)
	req, _ := http.NewRequest("POST", rig.server.URL+"/api/folders", strings.NewReader(`{"name":"hack"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "invalid-csrf-token")
	csrfResp, _ := acA.client.Do(req)
	csrfResp.Body.Close()
	if csrfResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid CSRF token, got %d", csrfResp.StatusCode)
	}

	// 3. User B cannot download User A's file (404)
	crossDlResp, _ := acB.client.Get(rig.server.URL + "/api/files/" + fileA.ID + "/download")
	crossDlResp.Body.Close()
	if crossDlResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user download, got %d", crossDlResp.StatusCode)
	}

	// 4. User B cannot trash User A's file
	var respMap map[string]bool
	acB.doJSON(t, "POST", rig.server.URL+"/api/nodes/trash", map[string][]string{"ids": {fileA.ID}}, &respMap)

	// Verify User A's file is still active and untouched
	var nodeCheck files.Node
	status := acA.doJSON(t, "GET", rig.server.URL+"/api/nodes/"+fileA.ID, nil, &nodeCheck)
	if status != http.StatusOK || nodeCheck.TrashedAt != nil {
		t.Fatalf("User A file was compromised by User B: %+v", nodeCheck)
	}
}

func TestPublicShareFileFlow(t *testing.T) {
	rig := setupTestRig(t)
	username := "alice"
	password := "Password123456"
	rig.createUser(t, username, password)
	ac := rig.login(t, username, password)

	// 1. Upload a file
	fileContent := "Hello AlphaDrive Public Sharing World!"
	node, _ := ac.uploadFile(t, rig.server.URL, ac.rootID, "shared_document.txt", fileContent)

	// 2. Create a public share for this file
	var shareResp struct {
		ID        string `json:"id"`
		NodeID    string `json:"node_id"`
		Slug      string `json:"slug"`
		PublicURL string `json:"public_url"`
	}
	createPayload := map[string]string{
		"node_id":     node.ID,
		"custom_slug": "my-cool-doc",
	}
	status := ac.doJSON(t, "POST", rig.server.URL+"/api/shares", createPayload, &shareResp)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 Created for share creation, got %d", status)
	}
	if shareResp.Slug != "my-cool-doc" {
		t.Fatalf("expected slug 'my-cool-doc', got %s", shareResp.Slug)
	}

	// 3. Public access to landing HTML page
	publicClient := &http.Client{}
	htmlResp, err := publicClient.Get(rig.server.URL + "/s/" + shareResp.Slug)
	if err != nil {
		t.Fatalf("GET public share page error: %v", err)
	}
	defer htmlResp.Body.Close()
	if htmlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for public share page, got %d", htmlResp.StatusCode)
	}

	// 4. Public access to view file
	viewResp, err := publicClient.Get(rig.server.URL + "/s/" + shareResp.Slug + "/files/" + node.ID + "/view")
	if err != nil {
		t.Fatalf("GET public file view error: %v", err)
	}
	viewBody, _ := io.ReadAll(viewResp.Body)
	viewResp.Body.Close()
	if viewResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for public view, got %d", viewResp.StatusCode)
	}
	if string(viewBody) != fileContent {
		t.Fatalf("expected view content %q, got %q", fileContent, string(viewBody))
	}

	// 5. Test Range request on public view
	rangeReq, _ := http.NewRequest("GET", rig.server.URL+"/s/"+shareResp.Slug+"/files/"+node.ID+"/view", nil)
	rangeReq.Header.Set("Range", "bytes=0-4")
	rangeResp, err := publicClient.Do(rangeReq)
	if err != nil {
		t.Fatalf("Range request error: %v", err)
	}
	rangeBody, _ := io.ReadAll(rangeResp.Body)
	rangeResp.Body.Close()
	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", rangeResp.StatusCode)
	}
	if string(rangeBody) != "Hello" {
		t.Fatalf("expected range body 'Hello', got %q", string(rangeBody))
	}

	// 6. Public download of file
	dlResp, err := publicClient.Get(rig.server.URL + "/s/" + shareResp.Slug + "/files/" + node.ID + "/download")
	if err != nil {
		t.Fatalf("GET public download error: %v", err)
	}
	defer dlResp.Body.Close()
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for public download, got %d", dlResp.StatusCode)
	}
	cd := dlResp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "shared_document.txt") {
		t.Fatalf("unexpected Content-Disposition header: %q", cd)
	}

	// 7. Revoke share
	revokeResp := map[string]bool{}
	delStatus := ac.doJSON(t, "DELETE", rig.server.URL+"/api/shares/"+shareResp.ID, nil, &revokeResp)
	if delStatus != http.StatusOK {
		t.Fatalf("expected 200 OK on revoke, got %d", delStatus)
	}

	// 8. Public access after revocation should be rejected
	postRevokeResp, _ := publicClient.Get(rig.server.URL + "/s/" + shareResp.Slug + "/files/" + node.ID + "/view")
	postRevokeResp.Body.Close()
	if postRevokeResp.StatusCode != http.StatusGone && postRevokeResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 410 or 404 after revocation, got %d", postRevokeResp.StatusCode)
	}
}

func TestPublicShareFolderAndZip(t *testing.T) {
	rig := setupTestRig(t)
	username := "bob"
	password := "Password123456"
	rig.createUser(t, username, password)
	ac := rig.login(t, username, password)

	// Create folder structure:
	// My Drive/
	//   SharedProject/
	//     report.txt ("report content")
	//     SubDir/
	//       notes.txt ("nested notes")
	var folder files.Node
	ac.doJSON(t, "POST", rig.server.URL+"/api/folders", map[string]string{"name": "SharedProject", "parent_id": ac.rootID}, &folder)

	file1, _ := ac.uploadFile(t, rig.server.URL, folder.ID, "report.txt", "report content")

	var subFolder files.Node
	ac.doJSON(t, "POST", rig.server.URL+"/api/folders", map[string]string{"name": "SubDir", "parent_id": folder.ID}, &subFolder)

	file2, _ := ac.uploadFile(t, rig.server.URL, subFolder.ID, "notes.txt", "nested notes")

	// Share SharedProject folder
	var shareResp struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	ac.doJSON(t, "POST", rig.server.URL+"/api/shares", map[string]string{"node_id": folder.ID}, &shareResp)

	publicClient := &http.Client{}

	// 1. Query nodes at root of share
	var nodesResp struct {
		Share struct {
			RootNode files.Node `json:"root_node"`
		} `json:"share"`
		Nodes []files.Node `json:"nodes"`
	}
	req, _ := http.NewRequest("GET", rig.server.URL+"/s/"+shareResp.Slug+"/nodes", nil)
	resp, err := publicClient.Do(req)
	if err != nil {
		t.Fatalf("GET shared nodes error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for shared nodes, got %d", resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(&nodesResp)
	resp.Body.Close()

	if len(nodesResp.Nodes) != 2 {
		t.Fatalf("expected 2 child nodes (file1, subFolder), got %d", len(nodesResp.Nodes))
	}

	// 2. Download ZIP of selected files via public endpoint
	zipBody := bytes.NewBufferString(`{"ids":["` + file1.ID + `","` + file2.ID + `"]}`)
	zipReq, _ := http.NewRequest("POST", rig.server.URL+"/s/"+shareResp.Slug+"/download", zipBody)
	zipReq.Header.Set("Content-Type", "application/json")
	zipResp, err := publicClient.Do(zipReq)
	if err != nil {
		t.Fatalf("POST shared zip error: %v", err)
	}
	defer zipResp.Body.Close()
	if zipResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for shared ZIP download, got %d", zipResp.StatusCode)
	}

	rawZip, err := io.ReadAll(zipResp.Body)
	if err != nil {
		t.Fatalf("read zip stream error: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(rawZip), int64(len(rawZip)))
	if err != nil {
		t.Fatalf("invalid zip archive: %v", err)
	}
	if len(zr.File) < 2 {
		t.Fatalf("expected at least 2 entries in zip, found %d", len(zr.File))
	}
}

func TestPublicSharePasswordProtection(t *testing.T) {
	rig := setupTestRig(t)
	username := "charlie"
	password := "Password123456"
	rig.createUser(t, username, password)
	ac := rig.login(t, username, password)

	node, _ := ac.uploadFile(t, rig.server.URL, ac.rootID, "protected.txt", "confidential")

	// Create password-protected share
	var shareResp struct {
		ID          string `json:"id"`
		Slug        string `json:"slug"`
		HasPassword bool   `json:"has_password"`
	}
	ac.doJSON(t, "POST", rig.server.URL+"/api/shares", map[string]string{
		"node_id":  node.ID,
		"password": "sharePassword999",
	}, &shareResp)

	if !shareResp.HasPassword {
		t.Fatalf("expected has_password to be true")
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 1. Unlocked access rejected (401)
	unauthResp, _ := client.Get(rig.server.URL + "/s/" + shareResp.Slug + "/files/" + node.ID + "/view")
	unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for password-protected share without grant, got %d", unauthResp.StatusCode)
	}

	// 2. Unlock with wrong password rejected (401)
	wrongUnlock, _ := client.Post(rig.server.URL+"/s/"+shareResp.Slug+"/unlock", "application/json", strings.NewReader(`{"password":"wrong"}`))
	wrongUnlock.Body.Close()
	if wrongUnlock.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for wrong share password, got %d", wrongUnlock.StatusCode)
	}

	// 3. Unlock with valid password succeeds and sets grant cookie
	validUnlock, err := client.Post(rig.server.URL+"/s/"+shareResp.Slug+"/unlock", "application/json", strings.NewReader(`{"password":"sharePassword999"}`))
	if err != nil {
		t.Fatalf("unlock request error: %v", err)
	}
	validUnlock.Body.Close()
	if validUnlock.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on correct share password unlock, got %d", validUnlock.StatusCode)
	}

	// 4. Access file view with grant cookie succeeds
	grantedResp, err := client.Get(rig.server.URL + "/s/" + shareResp.Slug + "/files/" + node.ID + "/view")
	if err != nil {
		t.Fatalf("view request error: %v", err)
	}
	grantedBody, _ := io.ReadAll(grantedResp.Body)
	grantedResp.Body.Close()
	if grantedResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after unlock, got %d", grantedResp.StatusCode)
	}
	if string(grantedBody) != "confidential" {
		t.Fatalf("expected body 'confidential', got %q", string(grantedBody))
	}
}

func TestHealthzAndDoctor(t *testing.T) {
	rig := setupTestRig(t)

	// Test healthz
	resp, err := http.Get(rig.server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", resp.StatusCode)
	}

	// Test doctor diagnostic run
	var docBuf bytes.Buffer
	ok := doctor.Run(context.Background(), rig.cfg, rig.db.DB, &docBuf)
	if !ok {
		t.Fatalf("expected doctor diagnostics to pass, output:\n%s", docBuf.String())
	}
}

func TestShareValidationAndStorageAPIs(t *testing.T) {
	rig := setupTestRig(t)
	username := "dave"
	password := "Password123456"
	rig.createUser(t, username, password)
	ac := rig.login(t, username, password)

	node, _ := ac.uploadFile(t, rig.server.URL, ac.rootID, "sample.txt", "data")

	// 1. Invalid Expiry -> 400
	var errResp map[string]any
	status := ac.doJSON(t, "POST", rig.server.URL+"/api/shares", map[string]string{
		"node_id": node.ID,
		"expiry":  "invalid-duration-format",
	}, &errResp)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid expiry, got %d", status)
	}

	// 2. Short Share Password (<12 chars) -> 400
	status = ac.doJSON(t, "POST", rig.server.URL+"/api/shares", map[string]string{
		"node_id":  node.ID,
		"password": "short",
	}, &errResp)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for short share password, got %d", status)
	}

	// 3. Invalid Slug (underscores) -> 400
	status = ac.doJSON(t, "POST", rig.server.URL+"/api/shares", map[string]string{
		"node_id":     node.ID,
		"custom_slug": "slug_with_underscore",
	}, &errResp)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for slug with underscore, got %d", status)
	}

	// 4. Valid Share with 24h Expiry -> 201
	var validShare struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	status = ac.doJSON(t, "POST", rig.server.URL+"/api/shares", map[string]string{
		"node_id":     node.ID,
		"custom_slug": "valid-slug-2026",
		"expiry":      "24h",
	}, &validShare)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for valid share, got %d", status)
	}

	// 5. Test /api/me storage telemetry fields
	var meResp struct {
		StorageDetectionAvailable bool   `json:"storage_detection_available"`
		AlphadriveUsedBytes       int64  `json:"alphadrive_used_bytes"`
		FilesystemTotalBytes      *int64 `json:"filesystem_total_bytes"`
	}
	ac.doJSON(t, "GET", rig.server.URL+"/api/me", nil, &meResp)
	if meResp.AlphadriveUsedBytes != 4 { // "data" = 4 bytes
		t.Fatalf("expected alphadrive_used_bytes=4, got %d", meResp.AlphadriveUsedBytes)
	}
}

func TestOwnerSetupFlow(t *testing.T) {
	rig := setupTestRig(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 1. Initial request to / should redirect to /setup (303 See Other)
	resp, err := client.Get(rig.server.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/setup" {
		t.Fatalf("expected 303 redirect to /setup, got status %d, loc %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// 2. GET /setup should return 200 with setup page and set CSRF cookie
	resp, err = client.Get(rig.server.URL + "/setup")
	if err != nil {
		t.Fatalf("get /setup: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /setup, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Owner Setup") {
		t.Fatalf("expected setup page body, got %s", string(body))
	}

	// Extract CSRF cookie
	serverURL, _ := url.Parse(rig.server.URL)
	var csrfToken string
	for _, c := range jar.Cookies(serverURL) {
		if c.Name == "alphadrive_login_csrf" {
			csrfToken = c.Value
		}
	}
	if csrfToken == "" {
		t.Fatalf("missing alphadrive_login_csrf cookie")
	}

	// 3. POST /setup with mismatched passwords should return 400
	form := url.Values{
		"csrf":             {csrfToken},
		"name":             {"System Administrator"},
		"username":         {"sysadmin"},
		"password":         {"SuperSecretPassword123!"},
		"confirm_password": {"DifferentPassword123!"},
	}
	resp, err = client.PostForm(rig.server.URL+"/setup", form)
	if err != nil {
		t.Fatalf("post /setup: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on mismatched password, got %d", resp.StatusCode)
	}

	// 4. POST /setup with password < 8 chars should return 400
	form.Set("password", "short")
	form.Set("confirm_password", "short")
	resp, err = client.PostForm(rig.server.URL+"/setup", form)
	if err != nil {
		t.Fatalf("post /setup: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on short password, got %d", resp.StatusCode)
	}

	// 5. POST /setup with valid credentials creates owner, sets session, and redirects to /
	form.Set("password", "SuperSecretPassword123!")
	form.Set("confirm_password", "SuperSecretPassword123!")
	resp, err = client.PostForm(rig.server.URL+"/setup", form)
	if err != nil {
		t.Fatalf("post /setup: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("expected 303 redirect to /, got status %d, loc %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// 6. Check authenticated session on /api/me
	req, _ := http.NewRequest("GET", rig.server.URL+"/api/me", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("get /api/me: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /api/me, got %d", resp.StatusCode)
	}
	var me struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if me.Username != "sysadmin" || me.Name != "System Administrator" {
		t.Fatalf("expected sysadmin with name 'System Administrator', got %+v", me)
	}

	// 7. GET /setup now that a user exists should redirect to / (since logged in)
	resp, err = client.Get(rig.server.URL + "/setup")
	if err != nil {
		t.Fatalf("get /setup: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("expected 303 redirect to / after setup, got %d loc %q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestAccountSettingsAPI(t *testing.T) {
	rig := setupTestRig(t)

	// Create an admin user and a regular user
	adminPass := "AdminMasterPass123!"
	adminHash, _ := auth.HashPassword(adminPass)
	srv := New(rig.cfg, rig.db.DB, rig.files, rig.shares)
	if err := srv.CreateUser(context.Background(), "adminuser", adminHash, true); err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	userPass := "RegularUserPass123!"
	userHash, _ := auth.HashPassword(userPass)
	if err := srv.CreateUser(context.Background(), "normaluser", userHash, false); err != nil {
		t.Fatalf("create normal user: %v", err)
	}

	adminClient := rig.login(t, "adminuser", adminPass)
	normalClient := rig.login(t, "normaluser", userPass)

	// 1. Username change test
	// 1a. Invalid username format
	status := normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/username", map[string]string{
		"new_username": "invalid user name with spaces",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid username format, got %d", status)
	}

	// 1b. Duplicate username
	status = normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/username", map[string]string{
		"new_username": "adminuser",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate username, got %d", status)
	}

	// 1c. Valid username change
	status = normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/username", map[string]string{
		"new_username": "renameduser",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for valid username change, got %d", status)
	}

	// Verify /api/me reflects renamed username
	meReq, _ := http.NewRequest("GET", rig.server.URL+"/api/me", nil)
	meResp, err := normalClient.client.Do(meReq)
	if err != nil {
		t.Fatalf("get /api/me: %v", err)
	}
	var meData struct {
		Username string `json:"username"`
		IsAdmin  bool   `json:"is_admin"`
	}
	_ = json.NewDecoder(meResp.Body).Decode(&meData)
	if meData.Username != "renameduser" {
		t.Fatalf("expected username 'renameduser', got %q", meData.Username)
	}
	if meData.IsAdmin != false {
		t.Fatalf("expected is_admin false for normal user")
	}

	// 2. Password change test
	// 2a. Wrong current password
	status = normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/password", map[string]string{
		"current_password": "WrongPassword123!",
		"new_password":     "BrandNewPassword123!",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong current password, got %d", status)
	}

	// 2b. New password too short
	status = normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/password", map[string]string{
		"current_password": userPass,
		"new_password":     "short",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for short new password, got %d", status)
	}

	// 2c. Successful password update
	newPassword := "BrandNewPassword123!"
	status = normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/password", map[string]string{
		"current_password": userPass,
		"new_password":     newPassword,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for valid password update, got %d", status)
	}

	// Verify login with new credentials succeeds
	newLoginClient := rig.login(t, "renameduser", newPassword)
	if newLoginClient == nil {
		t.Fatalf("expected successful login with new password")
	}

	// 3. User creation test
	// 3a. Normal user cannot create new users (403)
	status = normalClient.doJSON(t, "POST", rig.server.URL+"/api/account/users", map[string]any{
		"username": "thirduser",
		"password": "ThirdUserPassword123!",
	}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin user creation, got %d", status)
	}

	// 3b. Admin user creating user with short password (400)
	status = adminClient.doJSON(t, "POST", rig.server.URL+"/api/account/users", map[string]any{
		"username": "thirduser",
		"password": "short",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for short password in add user, got %d", status)
	}

	// 3c. Admin creates new user successfully (201)
	thirdPass := "ThirdUserPassword123!"
	status = adminClient.doJSON(t, "POST", rig.server.URL+"/api/account/users", map[string]any{
		"username": "thirduser",
		"name":     "Third Member",
		"password": thirdPass,
		"is_admin": false,
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for admin creating user, got %d", status)
	}

	// 3d. Duplicate user creation (409)
	status = adminClient.doJSON(t, "POST", rig.server.URL+"/api/account/users", map[string]any{
		"username": "thirduser",
		"password": thirdPass,
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate username creation, got %d", status)
	}

	// Verify newly created user can log in
	thirdClient := rig.login(t, "thirduser", thirdPass)
	if thirdClient == nil {
		t.Fatalf("expected successful login for newly created thirduser")
	}
}

func TestHTMLContentTypeRegression(t *testing.T) {
	rig := setupTestRig(t)

	// 1. Fresh instance: GET /setup must return Content-Type: text/html; charset=utf-8
	resp, err := http.Get(rig.server.URL + "/setup")
	if err != nil {
		t.Fatalf("GET /setup failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /setup, got %d", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	expectedCT := "text/html; charset=utf-8"
	if contentType != expectedCT {
		t.Fatalf("expected Content-Type %q for /setup, got %q", expectedCT, contentType)
	}

	// 2. POST /setup with error (400) must return Content-Type: text/html; charset=utf-8
	resp400, err := http.PostForm(rig.server.URL+"/setup", url.Values{
		"name": {""},
	})
	if err != nil {
		t.Fatalf("POST /setup failed: %v", err)
	}
	defer resp400.Body.Close()
	if resp400.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad POST /setup, got %d", resp400.StatusCode)
	}
	if ct := resp400.Header.Get("Content-Type"); ct != expectedCT {
		t.Fatalf("expected Content-Type %q for 400 /setup, got %q", expectedCT, ct)
	}

	// Create user so login is accessible
	rig.createUser(t, "testadmin", "ValidPassword123!")

	// 3. GET /login must return Content-Type: text/html; charset=utf-8
	loginResp, err := http.Get(rig.server.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login failed: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /login, got %d", loginResp.StatusCode)
	}
	if ct := loginResp.Header.Get("Content-Type"); ct != expectedCT {
		t.Fatalf("expected Content-Type %q for /login, got %q", expectedCT, ct)
	}

	// 4. POST /login with invalid credentials (401) must return Content-Type: text/html; charset=utf-8
	loginErrResp, err := http.PostForm(rig.server.URL+"/login", url.Values{
		"username": {"testadmin"},
		"password": {"WrongPassword!"},
		"csrf":     {"invalid_csrf"},
	})
	if err != nil {
		t.Fatalf("POST /login failed: %v", err)
	}
	defer loginErrResp.Body.Close()
	if ct := loginErrResp.Header.Get("Content-Type"); ct != expectedCT {
		t.Fatalf("expected Content-Type %q for invalid /login, got %q", expectedCT, ct)
	}

	// 5. Authenticated GET / (dashboard) must return Content-Type: text/html; charset=utf-8
	client := rig.login(t, "testadmin", "ValidPassword123!")
	dashResp, err := client.client.Get(rig.server.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer dashResp.Body.Close()
	if dashResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /, got %d", dashResp.StatusCode)
	}
	if ct := dashResp.Header.Get("Content-Type"); ct != expectedCT {
		t.Fatalf("expected Content-Type %q for GET /, got %q", expectedCT, ct)
	}

	// 6. Favicon should remain image/png
	favResp, err := http.Get(rig.server.URL + "/favicon.ico")
	if err != nil {
		t.Fatalf("GET /favicon.ico failed: %v", err)
	}
	defer favResp.Body.Close()
	if ct := favResp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected Content-Type image/png for /favicon.ico, got %q", ct)
	}

	// 7. /static/css/app.css must return text/css and contain all critical auth/setup selectors
	cssResp, err := http.Get(rig.server.URL + "/static/css/app.css")
	if err != nil {
		t.Fatalf("GET /static/css/app.css failed: %v", err)
	}
	defer cssResp.Body.Close()
	if cssResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /static/css/app.css, got %d", cssResp.StatusCode)
	}
	cssCT := cssResp.Header.Get("Content-Type")
	if !strings.HasPrefix(cssCT, "text/css") {
		t.Fatalf("expected text/css Content-Type for app.css, got %q", cssCT)
	}
	cssBody, err := io.ReadAll(cssResp.Body)
	if err != nil {
		t.Fatalf("read css body: %v", err)
	}
	cssStr := string(cssBody)
	criticalSelectors := []string{
		".auth-page",
		".auth-shell",
		".auth-logo",
		".auth-title",
		".auth-card",
		".form-group",
		".btn-submit",
	}
	for _, sel := range criticalSelectors {
		if !strings.Contains(cssStr, sel) {
			t.Fatalf("missing critical selector %q in served app.css", sel)
		}
	}

	// 8. /static/js/app.js must return javascript MIME
	jsResp, err := http.Get(rig.server.URL + "/static/js/app.js")
	if err != nil {
		t.Fatalf("GET /static/js/app.js failed: %v", err)
	}
	defer jsResp.Body.Close()
	if jsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /static/js/app.js, got %d", jsResp.StatusCode)
	}
	jsCT := jsResp.Header.Get("Content-Type")
	if !strings.Contains(jsCT, "javascript") && !strings.Contains(jsCT, "text/plain") {
		t.Fatalf("expected javascript Content-Type for app.js, got %q", jsCT)
	}

	// 9. /static/images/logo.png must return image/png
	imgResp, err := http.Get(rig.server.URL + "/static/images/logo.png")
	if err != nil {
		t.Fatalf("GET /static/images/logo.png failed: %v", err)
	}
	defer imgResp.Body.Close()
	if imgResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /static/images/logo.png, got %d", imgResp.StatusCode)
	}
	if ct := imgResp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected Content-Type image/png for logo.png, got %q", ct)
	}
}
