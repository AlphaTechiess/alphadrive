package system

import (
	"os"
	"testing"
)

func TestGetDiskStats(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}

	stats, err := GetDiskStats(wd)
	if err != nil {
		t.Fatalf("GetDiskStats failed: %v", err)
	}

	if stats.TotalBytes == 0 {
		t.Errorf("expected TotalBytes > 0, got 0")
	}
	if stats.TotalBytes < stats.FreeBytes {
		t.Errorf("expected TotalBytes >= FreeBytes, got total=%d free=%d", stats.TotalBytes, stats.FreeBytes)
	}
	t.Logf("Disk stats for %s: Total=%d, Used=%d, Free=%d", wd, stats.TotalBytes, stats.UsedBytes, stats.FreeBytes)
}
