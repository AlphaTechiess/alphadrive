# File & Storage Management

AlphaDrive provides an intuitive, high-performance file management platform designed to handle small documents and multi-gigabyte media files with equal ease.

---

## 1. Uploading Files and Folders

- **File Upload**: Click the floating action button (`+`) or header button and select **Upload files**, or drag and drop files directly onto the file grid.
- **Folder Upload**: Select **Upload folder** from the menu to upload entire folder structures preserving all nested directory hierarchies.
- **New Folder**: Click **New folder** to create an empty subfolder.

---

## 2. In-Browser Multi-Format Previews

Clicking on any file opens the instant media viewer:

- ðŸ–¼ï¸ **Images**: High-resolution image preview supporting PNG, JPG, JPEG, GIF, WebP, SVG, ICO, BMP.
- ðŸŽ¬ **Video Streaming**: Integrated HTML5 video player with HTTP Range request support for seamless seeking in MP4, WebM, MKV, MOV, and AVI.
- ðŸŽµ **Audio Player**: Dedicated waveform audio player for MP3, WAV, OGG, M4A, FLAC, AAC.
- ðŸ“„ **PDF Documents**: Embedded full-page interactive PDF viewer.
- ðŸ’» **Source Code & Text**: Syntax-styled text inspector for JSON, YAML, TOML, Go, Python, JS, TS, HTML, CSS, SQL, Shell, Markdown, and TXT files (up to 500 KB instant view).
- ðŸ“¦ **Binary / Unsupported**: Item details card with one-click direct download button.

---

## 3. Trash & Recovery Lifecycle

- **Moving to Trash**: Select one or more items and click the **Trash** icon in the bottom action bar.
- **Restoring Items**: Open the **Trash** view, select deleted files/folders, and click **Restore**. Items return to their original folder hierarchy.
- **Permanent Deletion**: Select items in Trash and click **Delete permanently**. This deletes the database metadata and wipes the underlying binary object blob from disk.

---

## 4. VPS Disk Storage Telemetry

The storage bar in the sidebar monitors your actual host VPS filesystem in real-time:
- **Total Disk Space**: Total physical disk partition size.
- **Used Space**: Overall disk space consumed across the server.
- **Free Space**: Remaining disk capacity available for uploads.
- **AlphaDrive Space**: Exact bytes consumed specifically by AlphaDrive files.
- Clicking the storage widget opens the **Storage Overview** modal with detailed storage statistics.