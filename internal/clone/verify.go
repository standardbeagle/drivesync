package clone

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"
)

// Verify compares source and destination to ensure they match.
func Verify(srcPath, dstPath string, size int64, blockSize int, progress chan<- Progress) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	dst, err := os.Open(dstPath)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	defer dst.Close()

	return VerifyReaders(src, dst, size, blockSize, progress)
}

// VerifyReaders compares two readers to ensure they contain identical data.
func VerifyReaders(src, dst io.Reader, size int64, blockSize int, progress chan<- Progress) error {
	if blockSize == 0 {
		blockSize = 64 * 1024
	}

	srcBuf := make([]byte, blockSize)
	dstBuf := make([]byte, blockSize)

	var verified int64
	startTime := time.Now()

	// Progress reporting ticker
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var progressUpdated int32
	go func() {
		for range ticker.C {
			atomic.StoreInt32(&progressUpdated, 1)
		}
	}()

	for verified < size {
		toRead := int64(blockSize)
		if verified+toRead > size {
			toRead = size - verified
		}

		// Read from source
		srcN, srcErr := io.ReadFull(src, srcBuf[:toRead])
		if srcErr != nil && srcErr != io.EOF && srcErr != io.ErrUnexpectedEOF {
			return fmt.Errorf("source read error at offset %d: %w", verified, srcErr)
		}

		// Read from destination
		dstN, dstErr := io.ReadFull(dst, dstBuf[:toRead])
		if dstErr != nil && dstErr != io.EOF && dstErr != io.ErrUnexpectedEOF {
			return fmt.Errorf("destination read error at offset %d: %w", verified, dstErr)
		}

		// Compare lengths
		if srcN != dstN {
			return fmt.Errorf("length mismatch at offset %d: source=%d, destination=%d", verified, srcN, dstN)
		}

		if srcN == 0 {
			break
		}

		// Compare data
		if !bytes.Equal(srcBuf[:srcN], dstBuf[:dstN]) {
			// Find first differing byte
			for i := 0; i < srcN; i++ {
				if srcBuf[i] != dstBuf[i] {
					return fmt.Errorf("data mismatch at offset %d (byte %d of block): source=%02x, destination=%02x",
						verified+int64(i), i, srcBuf[i], dstBuf[i])
				}
			}
		}

		verified += int64(srcN)

		// Send progress update
		if progress != nil && atomic.CompareAndSwapInt32(&progressUpdated, 1, 0) {
			elapsed := time.Since(startTime)
			avgSpeed := float64(verified) / elapsed.Seconds()

			var remaining time.Duration
			if avgSpeed > 0 {
				remaining = time.Duration(float64(size-verified)/avgSpeed) * time.Second
			}

			select {
			case progress <- Progress{
				Copied:      verified, // Using same field for consistency
				Total:       size,
				AvgSpeed:    avgSpeed,
				Elapsed:     elapsed,
				Remaining:   remaining,
				Verifying:   true,
				VerifyBytes: verified,
			}:
			default:
			}
		}
	}

	// Send final progress
	if progress != nil {
		elapsed := time.Since(startTime)
		select {
		case progress <- Progress{
			Copied:      verified,
			Total:       size,
			AvgSpeed:    float64(verified) / elapsed.Seconds(),
			Elapsed:     elapsed,
			Verifying:   true,
			VerifyBytes: verified,
		}:
		default:
		}
	}

	return nil
}
