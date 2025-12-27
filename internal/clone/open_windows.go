//go:build windows

package clone

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procCreateFileW    = kernel32.NewProc("CreateFileW")
	procCloseHandle    = kernel32.NewProc("CloseHandle")
)

const (
	GENERIC_READ             = 0x80000000
	GENERIC_WRITE            = 0x40000000
	FILE_SHARE_READ          = 0x00000001
	FILE_SHARE_WRITE         = 0x00000002
	OPEN_EXISTING            = 3
	FILE_FLAG_NO_BUFFERING   = 0x20000000
	FILE_FLAG_WRITE_THROUGH  = 0x80000000
	INVALID_HANDLE_VALUE     = ^uintptr(0)
)

// openForRead opens a file for reading with optional direct I/O (no buffering on Windows).
func openForRead(path string, directIO bool) (*os.File, error) {
	if !directIO {
		// Use standard os.Open for buffered I/O
		return os.Open(path)
	}

	// Use CreateFile with FILE_FLAG_NO_BUFFERING for direct I/O
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	flags := uint32(FILE_FLAG_NO_BUFFERING)

	handle, _, err := procCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		GENERIC_READ,
		FILE_SHARE_READ|FILE_SHARE_WRITE,
		0,
		OPEN_EXISTING,
		uintptr(flags),
		0,
	)

	if handle == INVALID_HANDLE_VALUE {
		// Fall back to buffered I/O if direct I/O fails
		return os.Open(path)
	}

	// Convert Windows HANDLE to os.File
	return os.NewFile(uintptr(handle), path), nil
}

// openForWrite opens a file for writing with optional direct I/O (no buffering on Windows).
func openForWrite(path string, directIO bool) (*os.File, error) {
	if !directIO {
		// Use standard os.OpenFile for buffered I/O
		return os.OpenFile(path, os.O_WRONLY, 0)
	}

	// Use CreateFile with FILE_FLAG_NO_BUFFERING for direct I/O
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	flags := uint32(FILE_FLAG_NO_BUFFERING | FILE_FLAG_WRITE_THROUGH)

	handle, _, err := procCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		GENERIC_WRITE,
		FILE_SHARE_READ|FILE_SHARE_WRITE,
		0,
		OPEN_EXISTING,
		uintptr(flags),
		0,
	)

	if handle == INVALID_HANDLE_VALUE {
		// Fall back to buffered I/O if direct I/O fails
		return os.OpenFile(path, os.O_WRONLY, 0)
	}

	// Convert Windows HANDLE to os.File
	return os.NewFile(uintptr(handle), path), nil
}
