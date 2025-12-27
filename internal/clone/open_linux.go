//go:build linux || darwin || freebsd

package clone

import (
	"os"
	"syscall"
)

// openForRead opens a file for reading with optional direct I/O.
func openForRead(path string, directIO bool) (*os.File, error) {
	flags := os.O_RDONLY
	if directIO {
		flags |= syscall.O_DIRECT
	}

	f, err := os.OpenFile(path, flags, 0)
	if err != nil && directIO {
		// Retry without O_DIRECT if it fails
		return os.Open(path)
	}
	return f, err
}

// openForWrite opens a file for writing with optional direct I/O.
func openForWrite(path string, directIO bool) (*os.File, error) {
	flags := os.O_WRONLY
	if directIO {
		flags |= syscall.O_DIRECT
	}

	f, err := os.OpenFile(path, flags, 0)
	if err != nil && directIO {
		// Retry without O_DIRECT if it fails
		return os.OpenFile(path, os.O_WRONLY, 0)
	}
	return f, err
}
