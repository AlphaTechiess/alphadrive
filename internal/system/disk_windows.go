//go:build windows

package system

import (
	"syscall"
	"unsafe"
)

type DiskStats struct {
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

func GetDiskStats(path string) (DiskStats, error) {
	kernel32, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return DiskStats{}, err
	}
	defer kernel32.Release()

	proc, err := kernel32.FindProc("GetDiskFreeSpaceExW")
	if err != nil {
		return DiskStats{}, err
	}

	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return DiskStats{}, err
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	r1, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r1 == 0 {
		return DiskStats{}, callErr
	}

	used := uint64(0)
	if totalNumberOfBytes > totalNumberOfFreeBytes {
		used = totalNumberOfBytes - totalNumberOfFreeBytes
	}

	return DiskStats{
		TotalBytes: totalNumberOfBytes,
		FreeBytes:  totalNumberOfFreeBytes,
		UsedBytes:  used,
	}, nil
}
