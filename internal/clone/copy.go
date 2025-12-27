package clone

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
	"unsafe"
)

// Clone copies data from source to destination with progress reporting.
func Clone(srcPath, dstPath string, opts Options, progress chan<- Progress) (*Result, error) {
	// Open source with platform-specific direct I/O handling
	src, err := openForRead(srcPath, opts.DirectIO)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	// Open destination with platform-specific direct I/O handling
	dst, err := openForWrite(dstPath, opts.DirectIO)
	if err != nil {
		return nil, fmt.Errorf("open destination: %w", err)
	}
	defer dst.Close()

	// Determine clone size
	cloneSize := opts.CloneBytes
	if cloneSize == 0 {
		fi, err := src.Stat()
		if err != nil {
			// Try seeking to end for block devices
			size, err := src.Seek(0, io.SeekEnd)
			if err != nil {
				return nil, fmt.Errorf("determine source size: %w", err)
			}
			cloneSize = size
			if _, err := src.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("seek to start: %w", err)
			}
		} else {
			cloneSize = fi.Size()
		}
	}

	return CloneReaders(src, dst, cloneSize, opts, progress)
}

// CloneReaders copies data between io.ReadWriteSeeker interfaces.
// This is useful for testing and for working with files.
func CloneReaders(src io.Reader, dst io.Writer, size int64, opts Options, progress chan<- Progress) (*Result, error) {
	if opts.BlockSize == 0 {
		opts.BlockSize = 64 * 1024
	}

	// Allocate aligned buffer for direct I/O
	buf := alignedBuffer(opts.BlockSize)

	result := &Result{}
	startTime := time.Now()
	var copied int64
	var lastProgressTime time.Time
	var lastProgressBytes int64

	// Progress reporting ticker
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Atomic flag for progress updates
	var progressUpdated int32

	go func() {
		for range ticker.C {
			atomic.StoreInt32(&progressUpdated, 1)
		}
	}()

	for copied < size {
		// Calculate how much to read
		toRead := int64(opts.BlockSize)
		if copied+toRead > size {
			toRead = size - copied
		}

		// Read block
		n, err := io.ReadFull(src, buf[:toRead])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("read error at offset %d: %w", copied, err)
		}
		if n == 0 {
			break
		}

		// Write block
		written, err := dst.Write(buf[:n])
		if err != nil {
			return nil, fmt.Errorf("write error at offset %d: %w", copied, err)
		}
		if written != n {
			return nil, fmt.Errorf("short write at offset %d: wrote %d of %d", copied, written, n)
		}

		copied += int64(n)

		// Send progress update
		if progress != nil && atomic.CompareAndSwapInt32(&progressUpdated, 1, 0) {
			now := time.Now()
			elapsed := now.Sub(startTime)

			var speed float64
			if !lastProgressTime.IsZero() {
				dt := now.Sub(lastProgressTime).Seconds()
				if dt > 0 {
					speed = float64(copied-lastProgressBytes) / dt
				}
			}

			avgSpeed := float64(copied) / elapsed.Seconds()

			var remaining time.Duration
			if avgSpeed > 0 {
				remaining = time.Duration(float64(size-copied)/avgSpeed) * time.Second
			}

			select {
			case progress <- Progress{
				Copied:    copied,
				Total:     size,
				Speed:     speed,
				AvgSpeed:  avgSpeed,
				Elapsed:   elapsed,
				Remaining: remaining,
			}:
			default:
				// Don't block if channel is full
			}

			lastProgressTime = now
			lastProgressBytes = copied
		}
	}

	// Sync destination
	if syncer, ok := dst.(interface{ Sync() error }); ok {
		if err := syncer.Sync(); err != nil {
			return nil, fmt.Errorf("sync destination: %w", err)
		}
	}

	elapsed := time.Since(startTime)
	result.Success = true
	result.BytesCopied = copied
	result.Duration = elapsed
	if elapsed.Seconds() > 0 {
		result.AvgSpeed = float64(copied) / elapsed.Seconds()
	}

	// Send final progress
	if progress != nil {
		select {
		case progress <- Progress{
			Copied:   copied,
			Total:    size,
			AvgSpeed: result.AvgSpeed,
			Elapsed:  elapsed,
		}:
		default:
		}
	}

	return result, nil
}

// alignedBuffer allocates a page-aligned buffer for direct I/O.
func alignedBuffer(size int) []byte {
	// Allocate extra space for alignment
	pageSize := 4096
	buf := make([]byte, size+pageSize)

	// Find the aligned offset
	offset := pageSize - (int(uintptr(unsafe.Pointer(&buf[0]))) % pageSize)
	if offset == pageSize {
		offset = 0
	}

	return buf[offset : offset+size]
}
