package fs

import "errors"

// ErrUnsupportedOS is returned when the operating system is not supported.
var ErrUnsupportedOS = errors.New("unsupported operating system for disk space check")
