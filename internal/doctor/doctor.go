package doctor

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/AlphaTechiess/alphadrive/internal/config"
	"github.com/AlphaTechiess/alphadrive/internal/system"
)

type CheckResult struct {
	Name    string
	Status  string // "OK", "WARN", "FAIL"
	Message string
}

func Run(ctx context.Context, cfg config.Config, db *sql.DB, w io.Writer) bool {
	if w == nil {
		w = os.Stdout
	}

	fmt.Fprintln(w, "==================================================")
	fmt.Fprintln(w, " AlphaDrive System & Integrity Diagnostics")
	fmt.Fprintln(w, "==================================================")
	fmt.Fprintf(w, "Data Directory: %s\n", cfg.DataDir)
	fmt.Fprintf(w, "Listen Address: %s\n", cfg.ListenAddress)
	fmt.Fprintln(w, "--------------------------------------------------")

	var results []CheckResult
	allPassed := true

	record := func(name, status, msg string) {
		results = append(results, CheckResult{Name: name, Status: status, Message: msg})
		symbol := "[OK]  "
		if status == "WARN" {
			symbol = "[WARN]"
		} else if status == "FAIL" {
			symbol = "[FAIL]"
			allPassed = false
		}
		fmt.Fprintf(w, "%s %-30s : %s\n", symbol, name, msg)
	}

	// 1. SQLite integrity check
	var integrityResult string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		record("Database Integrity", "FAIL", fmt.Sprintf("Query error: %v", err))
	} else if integrityResult == "ok" {
		record("Database Integrity", "OK", "Database is intact and corruption-free")
	} else {
		record("Database Integrity", "FAIL", fmt.Sprintf("Corruption detected: %s", integrityResult))
	}

	// 2. Foreign key check
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		record("Foreign Key Integrity", "FAIL", fmt.Sprintf("Query error: %v", err))
	} else {
		fkErrors := 0
		for rows.Next() {
			fkErrors++
		}
		rows.Close()
		if fkErrors == 0 {
			record("Foreign Key Integrity", "OK", "All foreign key relations are consistent")
		} else {
			record("Foreign Key Integrity", "FAIL", fmt.Sprintf("%d foreign key violations found", fkErrors))
		}
	}

	// 3. Schema Migrations
	var maxMigration sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT max(version) FROM schema_migrations").Scan(&maxMigration); err != nil {
		record("Schema Migrations", "FAIL", fmt.Sprintf("Failed to query schema_migrations: %v", err))
	} else if !maxMigration.Valid || maxMigration.Int64 < 3 {
		record("Schema Migrations", "WARN", fmt.Sprintf("Current migration version is %d (expected 3)", maxMigration.Int64))
	} else {
		record("Schema Migrations", "OK", fmt.Sprintf("Schema is up to date (version %d)", maxMigration.Int64))
	}

	// 4. Data Directory & Subdirectories
	objectsDir := filepath.Join(cfg.DataDir, "files", "objects")
	tempDir := filepath.Join(cfg.DataDir, "files", "temp")
	for _, dir := range []struct{ name, path string }{
		{"Data Directory", cfg.DataDir},
		{"Objects Directory", objectsDir},
		{"Temp Directory", tempDir},
	} {
		if fi, err := os.Stat(dir.path); err != nil {
			record(dir.name, "FAIL", fmt.Sprintf("Directory missing: %v", err))
		} else if !fi.IsDir() {
			record(dir.name, "FAIL", "Path is not a directory")
		} else {
			record(dir.name, "OK", fmt.Sprintf("Exists (%s)", dir.path))
		}
	}

	// 5. Storage Write/Readability Probe
	probeFile := filepath.Join(tempDir, fmt.Sprintf(".doctor-probe-%d", time.Now().UnixNano()))
	if err := os.WriteFile(probeFile, []byte("alphadrive-probe"), 0600); err != nil {
		record("Storage Write Probe", "FAIL", fmt.Sprintf("Failed to write test file: %v", err))
	} else {
		data, err := os.ReadFile(probeFile)
		_ = os.Remove(probeFile)
		if err != nil || string(data) != "alphadrive-probe" {
			record("Storage Write Probe", "FAIL", "Failed to read back test file")
		} else {
			record("Storage Write Probe", "OK", "Read and write test succeeded")
		}
	}

	// 6. Missing Physical Files Check (DB -> Disk)
	dbRows, err := db.QueryContext(ctx, "SELECT id, name, storage_key FROM nodes WHERE kind='file'")
	knownStorageKeys := make(map[string]bool)
	missingFilesCount := 0
	totalFilesCount := 0

	if err != nil {
		record("Missing Files Check", "FAIL", fmt.Sprintf("Failed to query nodes: %v", err))
	} else {
		defer dbRows.Close()
		for dbRows.Next() {
			var id, name, key string
			if err := dbRows.Scan(&id, &name, &key); err != nil {
				continue
			}
			totalFilesCount++
			if key != "" {
				knownStorageKeys[key] = true
				objPath := filepath.Join(objectsDir, key)
				if _, err := os.Stat(objPath); os.IsNotExist(err) {
					missingFilesCount++
				}
			}
		}
		if missingFilesCount == 0 {
			record("Missing Files Check", "OK", fmt.Sprintf("All %d database file records have physical storage objects", totalFilesCount))
		} else {
			record("Missing Files Check", "FAIL", fmt.Sprintf("%d out of %d file records missing from storage", missingFilesCount, totalFilesCount))
		}
	}

	// 7. Orphaned Files Check (Disk -> DB)
	orphanedCount := 0
	totalDiskObjects := 0
	if entries, err := os.ReadDir(objectsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				totalDiskObjects++
				if !knownStorageKeys[e.Name()] {
					orphanedCount++
				}
			}
		}
		if orphanedCount == 0 {
			record("Orphaned Files Check", "OK", fmt.Sprintf("All %d disk storage objects belong to active database nodes", totalDiskObjects))
		} else {
			record("Orphaned Files Check", "WARN", fmt.Sprintf("%d orphaned disk objects found in %s", orphanedCount, objectsDir))
		}
	} else {
		record("Orphaned Files Check", "FAIL", fmt.Sprintf("Failed to scan objects directory: %v", err))
	}

	// 8. Filesystem Space Check
	diskStats, err := system.GetDiskStats(cfg.DataDir)
	if err != nil {
		record("Filesystem Storage", "WARN", fmt.Sprintf("Unable to probe disk stats: %v", err))
	} else {
		totalGB := float64(diskStats.TotalBytes) / (1024 * 1024 * 1024)
		freeGB := float64(diskStats.FreeBytes) / (1024 * 1024 * 1024)
		usedGB := float64(diskStats.UsedBytes) / (1024 * 1024 * 1024)
		record("Filesystem Storage", "OK", fmt.Sprintf("%.2f GB Total, %.2f GB Used, %.2f GB Free", totalGB, usedGB, freeGB))
	}

	// 9. Aggregate Application Stats
	var userCount, shareCount, sessionCount int
	var totalStorageBytes int64
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&userCount)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM shares WHERE revoked_at IS NULL").Scan(&shareCount)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE revoked_at IS NULL AND expires_at > unixepoch()").Scan(&sessionCount)
	_ = db.QueryRowContext(ctx, "SELECT coalesce(sum(size_bytes), 0) FROM nodes WHERE kind='file' AND trashed_at IS NULL").Scan(&totalStorageBytes)

	fmt.Fprintln(w, "--------------------------------------------------")
	fmt.Fprintf(w, "Summary Statistics:\n")
	fmt.Fprintf(w, "  Users              : %d\n", userCount)
	fmt.Fprintf(w, "  Active Files       : %d (%.2f MB)\n", totalFilesCount, float64(totalStorageBytes)/(1024*1024))
	fmt.Fprintf(w, "  Active Public Shares: %d\n", shareCount)
	fmt.Fprintf(w, "  Active Sessions    : %d\n", sessionCount)
	fmt.Fprintln(w, "==================================================")
	if allPassed {
		fmt.Fprintln(w, " Result: HEALTHY - All essential diagnostics passed")
	} else {
		fmt.Fprintln(w, " Result: ATTENTION - One or more critical checks failed")
	}
	fmt.Fprintln(w, "==================================================")

	return allPassed
}
