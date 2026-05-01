//go:build integration
// +build integration

package clone

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"testing"
)

// Integration tests for clone operations
// Run with: go test -tags=integration ./internal/clone/...

func TestCloneWithTempFiles(t *testing.T) {
	// Create temp source file with random data
	src, err := os.CreateTemp("", "drivesync-test-src-*")
	if err != nil {
		t.Fatalf("Failed to create source temp file: %v", err)
	}
	defer os.Remove(src.Name())
	defer src.Close()

	// Write 1MB of random data
	data := make([]byte, 1024*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Failed to generate random data: %v", err)
	}
	if _, err := src.Write(data); err != nil {
		t.Fatalf("Failed to write source data: %v", err)
	}
	src.Seek(0, 0)

	// Create temp destination file
	dst, err := os.CreateTemp("", "drivesync-test-dst-*")
	if err != nil {
		t.Fatalf("Failed to create dest temp file: %v", err)
	}
	defer os.Remove(dst.Name())
	defer dst.Close()

	// Clone
	opts := DefaultOptions()
	opts.BlockSize = 64 * 1024

	result, err := CloneReaders(src, dst, int64(len(data)), opts, nil)
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}

	t.Logf("Cloned %d bytes at %.2f MB/s", result.BytesCopied, result.AvgSpeedMBps())

	// Verify
	src.Seek(0, 0)
	dst.Seek(0, 0)

	srcData, _ := io.ReadAll(src)
	dstData, _ := io.ReadAll(dst)

	if !bytes.Equal(srcData, dstData) {
		t.Error("Source and destination data do not match")
	}
}

func TestVerifyWithTempFiles(t *testing.T) {
	// Create two identical temp files
	data := make([]byte, 512*1024) // 512KB
	rand.Read(data)

	file1, _ := os.CreateTemp("", "drivesync-verify-1-*")
	file2, _ := os.CreateTemp("", "drivesync-verify-2-*")
	defer os.Remove(file1.Name())
	defer os.Remove(file2.Name())
	defer file1.Close()
	defer file2.Close()

	file1.Write(data)
	file2.Write(data)
	file1.Seek(0, 0)
	file2.Seek(0, 0)

	blockSize := 64 * 1024
	err := VerifyReaders(file1, file2, int64(len(data)), blockSize, nil)
	if err != nil {
		t.Fatalf("Verify failed (should have matched): %v", err)
	}

	// Now corrupt one byte and verify again
	file2.Seek(1000, 0)
	file2.Write([]byte{0xFF})
	file1.Seek(0, 0)
	file2.Seek(0, 0)

	err = VerifyReaders(file1, file2, int64(len(data)), blockSize, nil)
	if err == nil {
		t.Error("Corrupted files should not verify as matching")
	}
}

func TestCloneLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Test with 100MB file
	size := int64(100 * 1024 * 1024)

	src, _ := os.CreateTemp("", "drivesync-large-src-*")
	dst, _ := os.CreateTemp("", "drivesync-large-dst-*")
	defer os.Remove(src.Name())
	defer os.Remove(dst.Name())
	defer src.Close()
	defer dst.Close()

	// Write random data in chunks
	written := int64(0)
	chunk := make([]byte, 1024*1024)
	for written < size {
		rand.Read(chunk)
		n, _ := src.Write(chunk)
		written += int64(n)
	}
	src.Seek(0, 0)

	opts := DefaultOptions()
	opts.BlockSize = 1024 * 1024 // 1MB blocks

	result, err := CloneReaders(src, dst, size, opts, nil)
	if err != nil {
		t.Fatalf("Large clone failed: %v", err)
	}

	t.Logf("Cloned %d MB at %.2f MB/s in %v",
		result.BytesCopied/1024/1024,
		result.AvgSpeedMBps(),
		result.Duration)
}
