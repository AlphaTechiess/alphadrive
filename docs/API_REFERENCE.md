# AlphaDrive REST API Reference

AlphaDrive provides a clean, JSON-based REST API for programmatic access, client automation, and integration.

---

## ðŸ”‘ Authentication & Headers

- **Session Cookie**: `alphadrive_session=<token>` (HTTP-only, SameSite=Lax).
- **CSRF Header**: For state-changing requests (`POST`, `DELETE`, `PUT`), include:
  `X-CSRF-Token: <csrf_token>`

---

## ðŸ“‹ API Endpoints

### 1. User & Account
- `GET /api/me` â€” Returns authenticated user details and storage quota.
- `POST /api/account/username` â€” Change personal username (`{ "new_username": "..." }`).
- `POST /api/account/password` â€” Change password (`{ "current_password": "...", "new_password": "..." }`).
- `POST /api/account/users` *(Admin only)* â€” Create new user (`{ "username": "...", "name": "...", "password": "...", "is_admin": bool }`).

### 2. Files & Folders
- `GET /api/nodes?parent_id=<id>` â€” List active files and folders inside a directory.
- `GET /api/nodes/trash` â€” List items currently in trash.
- `GET /api/nodes/{id}` â€” Get metadata for a specific item.
- `POST /api/nodes/folder` â€” Create a new folder (`{ "name": "...", "parent_id": "..." }`).
- `POST /api/nodes/trash` â€” Move items to trash (`{ "ids": ["id1", "id2"] }`).
- `POST /api/nodes/restore` â€” Restore items from trash (`{ "ids": ["id1", "id2"] }`).
- `DELETE /api/nodes` â€” Permanently delete items (`{ "ids": ["id1", "id2"] }`).

### 3. File Transfers & Streaming
- `POST /api/upload` â€” Multi-part file upload (`file`, `parent_id`, `relative_path`).
- `GET /api/files/{id}/view` â€” Stream file content for browser preview (Supports Range headers).
- `GET /api/files/{id}/download` â€” Download file attachment.
- `GET /api/folders/{id}/download` â€” Download folder contents as a streamed ZIP archive.

### 4. Public Shares
- `GET /api/nodes/{id}/share` â€” Get active share status for an item.
- `POST /api/shares` â€” Create a public share link (`{ "node_id": "...", "custom_slug": "...", "password": "...", "expires_in": "24h" }`).
- `DELETE /api/shares/{id}` â€” Revoke an active public share link.
- `GET /s/{slug}` â€” Public share web page.
- `POST /s/{slug}/unlock` â€” Unlock password-protected public share.
- `GET /s/{slug}/files/{id}/view` â€” Public preview stream.
- `GET /s/{slug}/files/{id}/download` â€” Public file download.
- `GET /s/{slug}/download` â€” Public folder ZIP stream.