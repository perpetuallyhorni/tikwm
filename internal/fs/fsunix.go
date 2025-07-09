//go:build linux || darwin || freebsd || openbsd || netbsd

package fs

import "golang.org/x/sys/unix"

// Available returns the number of bytes available to the user (non-root) on the filesystem.
func Available(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Available blocks * block size
	return stat.Bavail * uint64(stat.Bsize), nil // #nosec G115
}
