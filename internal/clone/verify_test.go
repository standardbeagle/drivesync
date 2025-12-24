package clone

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestVerifyReaders(t *testing.T) {
	// Create identical data for source and destination
	testData := make([]byte, 1024*1024) // 1MB
	if _, err := rand.Read(testData); err != nil {
		t.Fatalf("failed to generate test data: %v", err)
	}

	src := bytes.NewReader(testData)
	dst := bytes.NewReader(testData) // Same data

	err := VerifyReaders(src, dst, int64(len(testData)), 64*1024, nil)
	if err != nil {
		t.Errorf("VerifyReaders failed on identical data: %v", err)
	}
}

func TestVerifyReadersMismatch(t *testing.T) {
	srcData := make([]byte, 1024*1024)
	dstData := make([]byte, 1024*1024)

	// Fill with different patterns
	for i := range srcData {
		srcData[i] = byte(i % 256)
		dstData[i] = byte(i % 256)
	}

	// Introduce a difference
	dstData[500000] = srcData[500000] + 1

	src := bytes.NewReader(srcData)
	dst := bytes.NewReader(dstData)

	err := VerifyReaders(src, dst, int64(len(srcData)), 64*1024, nil)
	if err == nil {
		t.Error("Expected error on data mismatch")
	}
}

func TestVerifyReadersSmallData(t *testing.T) {
	testData := []byte("Hello, DriveSync!")

	src := bytes.NewReader(testData)
	dst := bytes.NewReader(testData)

	err := VerifyReaders(src, dst, int64(len(testData)), 4, nil)
	if err != nil {
		t.Errorf("VerifyReaders failed: %v", err)
	}
}

func TestVerifyReadersWithProgress(t *testing.T) {
	testData := make([]byte, 256*1024) // 256KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	src := bytes.NewReader(testData)
	dst := bytes.NewReader(testData)

	progressChan := make(chan Progress, 100)
	done := make(chan struct{})

	var progressUpdates []Progress
	go func() {
		for p := range progressChan {
			progressUpdates = append(progressUpdates, p)
		}
		close(done)
	}()

	err := VerifyReaders(src, dst, int64(len(testData)), 16*1024, progressChan)
	close(progressChan)
	<-done

	if err != nil {
		t.Errorf("VerifyReaders failed: %v", err)
	}

	// Verify progress was reported with Verifying flag
	for _, p := range progressUpdates {
		if !p.Verifying {
			t.Error("Progress should have Verifying flag set")
		}
	}
}

func TestVerifyReadersLengthMismatch(t *testing.T) {
	srcData := make([]byte, 1000)
	dstData := make([]byte, 500) // Shorter

	src := bytes.NewReader(srcData)
	dst := bytes.NewReader(dstData)

	err := VerifyReaders(src, dst, int64(len(srcData)), 64*1024, nil)
	if err == nil {
		t.Error("Expected error on length mismatch")
	}
}

func TestVerifyReadersEmpty(t *testing.T) {
	src := bytes.NewReader([]byte{})
	dst := bytes.NewReader([]byte{})

	err := VerifyReaders(src, dst, 0, 64*1024, nil)
	if err != nil {
		t.Errorf("VerifyReaders failed on empty data: %v", err)
	}
}

func TestVerifyReadersFirstByteDifference(t *testing.T) {
	srcData := []byte{0x00, 0x01, 0x02, 0x03}
	dstData := []byte{0xFF, 0x01, 0x02, 0x03} // First byte different

	src := bytes.NewReader(srcData)
	dst := bytes.NewReader(dstData)

	err := VerifyReaders(src, dst, int64(len(srcData)), 64*1024, nil)
	if err == nil {
		t.Error("Expected error on first byte difference")
	}

	// Error message should indicate offset 0
	if err != nil && !bytes.Contains([]byte(err.Error()), []byte("offset 0")) {
		t.Errorf("Error should mention offset 0: %v", err)
	}
}

func TestVerifyReadersLastByteDifference(t *testing.T) {
	size := 1024
	srcData := make([]byte, size)
	dstData := make([]byte, size)

	for i := range srcData {
		srcData[i] = byte(i % 256)
		dstData[i] = byte(i % 256)
	}

	// Last byte different
	dstData[size-1] = srcData[size-1] + 1

	src := bytes.NewReader(srcData)
	dst := bytes.NewReader(dstData)

	err := VerifyReaders(src, dst, int64(size), 64*1024, nil)
	if err == nil {
		t.Error("Expected error on last byte difference")
	}
}

func TestVerifyReadersMultipleBlocks(t *testing.T) {
	// Create data that spans multiple blocks with difference in middle block
	blockSize := 1024
	numBlocks := 10
	size := blockSize * numBlocks

	srcData := make([]byte, size)
	dstData := make([]byte, size)

	for i := range srcData {
		srcData[i] = byte(i % 256)
		dstData[i] = byte(i % 256)
	}

	// Difference in block 5
	diffOffset := 5*blockSize + 100
	dstData[diffOffset] = srcData[diffOffset] + 1

	src := bytes.NewReader(srcData)
	dst := bytes.NewReader(dstData)

	err := VerifyReaders(src, dst, int64(size), blockSize, nil)
	if err == nil {
		t.Error("Expected error on block 5 difference")
	}
}
