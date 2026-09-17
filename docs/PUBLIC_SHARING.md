# Public Sharing Engine

AlphaDrive includes an advanced public link sharing platform that lets you share individual files or entire folders securely with anyone.

---

## 1. Creating a Public Share Link

1. Select any file or folder in **My Drive**.
2. Click the **Link** icon in the bottom action bar.
3. In the share modal, customize your share settings:
   - **Custom link slug (optional)**: Enter a vanity URL slug (e.g. `project-deck` or leave empty for an auto-generated random slug.
   - **Protect with password (optional)**: Set an Argon2id password (minimum 12 characters) to restrict access.
   - **Expires after (optional)**: Choose `Never expires`, `1 hour`, `24 hours`, `7 days`, or `30 days`.
4. Click **Create link**.
5. The public link is generated and automatically copied to your clipboard (e.g. `https://drive.example.com/s/project-deck`).

---

## 2. Managing & Revoking Links

When you open the share modal for an item with an active link:
- Displays public URL with one-click **Copy** and **Open link** buttons.
- Shows total view count, expiration date, and password protection status.
- **Revoke Link**: Click **Revoke link** to instantly destroy public access. Anyone attempting to visit the URL thereafter receives an expired/revoked message.

---

## 3. Public Recipient Experience

- **Centered Branding**: Clean, responsive public header featuring the centered AlphaDrive brand identity.
- **Password Protection**: If password-protected, prompts the visitor for the unlock password before revealing contents.
- **In-Browser Previews**: Recipients can preview images, stream video/audio, and inspect PDFs directly in the browser without downloading.
- **Folder Streaming & Batch ZIP**: When sharing a folder, recipients can navigate subfolders or click **Download all** to download the entire folder as a dynamically streamed ZIP archive.