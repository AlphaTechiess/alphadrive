package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BackupInfo struct {
	Version      string    `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	DatabaseSize int64     `json:"database_size"`
	ObjectCount  int       `json:"object_count"`
	TotalBytes   int64     `json:"total_bytes"`
}

// Create generates a complete, atomic tar.gz backup of the SQLite database and storage objects
func Create(ctx context.Context, db *sql.DB, dataDir, outputPath string) error {
	if outputPath == "" {
		timestamp := time.Now().UTC().Format("20060102-150405")
		outputPath = fmt.Sprintf("alphadrive-backup-%s.tar.gz", timestamp)
	}

	// Ensure output directory exists
	outDir := filepath.Dir(outputPath)
	if outDir != "" && outDir != "." {
		if err := os.MkdirAll(outDir, 0750); err != nil {
			return fmt.Errorf("create backup output directory: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp("", "alphadrive-backup-*")
	if err != nil {
		return fmt.Errorf("create temp staging dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	snapshotDBPath := filepath.Join(tempDir, "alphadrive.db")

	// 1. Create an atomic SQLite snapshot using SQLite's VACUUM INTO
	// Format path with forward slashes for SQLite string literal
	escapedSnapshotPath := strings.ReplaceAll(snapshotDBPath, `\`, `/`)
	_, err = db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", escapedSnapshotPath))
	if err != nil {
		// Fallback: WAL checkpoint and atomic file copy if VACUUM INTO is restricted
		_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
		srcDB := filepath.Join(dataDir, "alphadrive.db")
		if copyErr := copyFile(srcDB, snapshotDBPath); copyErr != nil {
			return fmt.Errorf("database backup failed: vacuum err: %v, fallback copy err: %w", err, copyErr)
		}
	}

	dbStat, err := os.Stat(snapshotDBPath)
	if err != nil {
		return fmt.Errorf("stat snapshot database: %w", err)
	}

	// 2. Count objects and compute size
	objectsDir := filepath.Join(dataDir, "files", "objects")
	var objectCount int
	var objectsTotalBytes int64

	if entries, err := os.ReadDir(objectsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				if fi, err := e.Info(); err == nil {
					objectCount++
					objectsTotalBytes += fi.Size()
				}
			}
		}
	}

	info := BackupInfo{
		Version:      "1.0.0",
		CreatedAt:    time.Now().UTC(),
		DatabaseSize: dbStat.Size(),
		ObjectCount:  objectCount,
		TotalBytes:   dbStat.Size() + objectsTotalBytes,
	}

	// 3. Write compressed tar.gz archive
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output archive: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Write metadata.json
	metaJSON, _ := json.MarshalIndent(info, "", "  ")
	if err := writeTarEntry(tw, "metadata.json", metaJSON, info.CreatedAt); err != nil {
		return fmt.Errorf("write metadata entry: %w", err)
	}

	// Write alphadrive.db
	dbData, err := os.ReadFile(snapshotDBPath)
	if err != nil {
		return fmt.Errorf("read snapshot db: %w", err)
	}
	if err := writeTarEntry(tw, "alphadrive.db", dbData, info.CreatedAt); err != nil {
		return fmt.Errorf("write database entry: %w", err)
	}

	// Write all storage objects
	if entries, err := os.ReadDir(objectsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				objPath := filepath.Join(objectsDir, e.Name())
				data, err := os.ReadFile(objPath)
				if err != nil {
					continue
				}
				tarPath := "files/objects/" + e.Name()
				if err := writeTarEntry(tw, tarPath, data, info.CreatedAt); err != nil {
					return fmt.Errorf("write storage object %s: %w", e.Name(), err)
				}
			}
		}
	}

	return nil
}

// Restore unpacks a valid AlphaDrive tar.gz backup into the target data directory
func Restore(ctx context.Context, archivePath, targetDataDir string) (*BackupInfo, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open backup archive: %w", err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Ensure destination directory structure
	objectsDir := filepath.Join(targetDataDir, "files", "objects")
	tempDir := filepath.Join(targetDataDir, "files", "temp")
	if err := os.MkdirAll(objectsDir, 0700); err != nil {
		return nil, fmt.Errorf("create objects dir: %w", err)
	}
	if err := os.MkdirAll(tempDir, 0700); err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	var info BackupInfo
	foundDB := false

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive entry: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "\\") {
			return nil, fmt.Errorf("suspicious file path in archive: %s", hdr.Name)
		}

		switch cleanName {
		case "metadata.json":
			var metaData bytes.Buffer
			if _, err := io.Copy(&metaData, tr); err == nil {
				_ = json.Unmarshal(metaData.Bytes(), &info)
			}
		case "alphadrive.db":
			foundDB = true
			destPath := filepath.Join(targetDataDir, "alphadrive.db")
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				return nil, fmt.Errorf("create restored db: %w", err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return nil, fmt.Errorf("write restored db: %w", err)
			}
			out.Close()
		default:
			if strings.HasPrefix(cleanName, "files/objects/") || strings.HasPrefix(cleanName, "files\\objects\\") {
				objName := filepath.Base(cleanName)
				if objName == "" || objName == "." || objName == "/" || objName == "\\" {
					continue
				}
				destPath := filepath.Join(objectsDir, objName)
				out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
				if err != nil {
					return nil, fmt.Errorf("create restored object %s: %w", objName, err)
				}
				if _, err := io.Copy(out, tr); err != nil {
					out.Close()
					return nil, fmt.Errorf("write restored object %s: %w", objName, err)
				}
				out.Close()
			}
		}
	}

	if !foundDB {
		return nil, fmt.Errorf("archive does not contain alphadrive.db")
	}

	return &info, nil
}

func writeTarEntry(tw *tar.Writer, name string, content []byte, modTime time.Time) error {
	hdr := &tar.Header{
		Name:     name,
		Mode:     0600,
		Size:     int64(len(content)),
		ModTime:  modTime,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(content)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
