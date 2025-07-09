//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !windows

package fs

// Available returns an error as this OS is not supported.
func Available(path string) (uint64, error) {
	return 0, ErrUnsupportedOS
}
