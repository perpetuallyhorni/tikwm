//go:build windows

package fs

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Available returns the number of bytes available to the user on the filesystem.
func Available(path string) (uint64, error) {
	var freeBytes int64
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	err = windows.GetDiskFreeSpaceEx(pathPtr, (*uint64)(unsafe.Pointer(&freeBytes)), nil, nil)
	if err != nil {
		return 0, err
	}
	return uint64(freeBytes), nil
}
