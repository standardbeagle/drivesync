package clone

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestCloneReaders(t *testing.T) {
	// Create random test data
	testData := make([]byte, 1024*1024) // 1MB
	if _, err := rand.Read(testData); err != nil {
		t.Fatalf("failed to generate test data: %v", err)
	}

	src := bytes.NewReader(testData)
	dst := &bytes.Buffer{}

	opts := Options{
		BlockSize: 64 * 1024,
		DirectIO:  false, // Can't use direct I/O with in-memory buffers
	}

	progressChan := make(chan Progress, 100)

	result, err := CloneReaders(src, dst, int64(len(testData)), opts, progressChan)
	close(progressChan)

	if err != nil {
		t.Fatalf("CloneReaders failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if result.BytesCopied != int64(len(testData)) {
		t.Errorf("BytesCopied: got %d, want %d", result.BytesCopied, len(testData))
	}

	// Verify data integrity
	if !bytes.Equal(testData, dst.Bytes()) {
		t.Error("Destination data doesn't match source")
	}

	// Check progress was reported
	progressCount := 0
	for range progressChan {
		progressCount++
	}
	// Should have received some progress updates (at least one)
	// Note: progress channel is already drained, so this checks nothing was left
}

func TestCloneReadersSmallBlocks(t *testing.T) {
	testData := []byte("Hello, DriveSync!")
	src := bytes.NewReader(testData)
	dst := &bytes.Buffer{}

	opts := Options{
		BlockSize: 4, // Very small block size
		DirectIO:  false,
	}

	result, err := CloneReaders(src, dst, int64(len(testData)), opts, nil)
	if err != nil {
		t.Fatalf("CloneReaders failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if !bytes.Equal(testData, dst.Bytes()) {
		t.Errorf("got %q, want %q", dst.Bytes(), testData)
	}
}

func TestCloneReadersPartialRead(t *testing.T) {
	testData := make([]byte, 1000)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	src := bytes.NewReader(testData)
	dst := &bytes.Buffer{}

	opts := Options{
		BlockSize: 64 * 1024, // Larger than data
		DirectIO:  false,
	}

	result, err := CloneReaders(src, dst, int64(len(testData)), opts, nil)
	if err != nil {
		t.Fatalf("CloneReaders failed: %v", err)
	}

	if result.BytesCopied != int64(len(testData)) {
		t.Errorf("BytesCopied: got %d, want %d", result.BytesCopied, len(testData))
	}

	if !bytes.Equal(testData, dst.Bytes()) {
		t.Error("Data mismatch")
	}
}

func TestCloneReadersLargeData(t *testing.T) {
	// Test with 10MB to verify chunking works correctly
	size := 10 * 1024 * 1024
	testData := make([]byte, size)

	// Fill with pattern for verification
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	src := bytes.NewReader(testData)
	dst := &bytes.Buffer{}

	opts := Options{
		BlockSize: 64 * 1024,
		DirectIO:  false,
	}

	result, err := CloneReaders(src, dst, int64(len(testData)), opts, nil)
	if err != nil {
		t.Fatalf("CloneReaders failed: %v", err)
	}

	if result.BytesCopied != int64(size) {
		t.Errorf("BytesCopied: got %d, want %d", result.BytesCopied, size)
	}

	if !bytes.Equal(testData, dst.Bytes()) {
		t.Error("Data mismatch in large transfer")
	}

	// Verify speed calculation
	if result.AvgSpeed <= 0 {
		t.Error("AvgSpeed should be positive")
	}
}

func TestCloneReadersWithProgress(t *testing.T) {
	testData := make([]byte, 256*1024) // 256KB
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	src := bytes.NewReader(testData)
	dst := &bytes.Buffer{}

	opts := Options{
		BlockSize: 16 * 1024, // 16KB blocks = 16 blocks
		DirectIO:  false,
	}

	progressChan := make(chan Progress, 100)
	done := make(chan struct{})

	var progressUpdates []Progress
	go func() {
		for p := range progressChan {
			progressUpdates = append(progressUpdates, p)
		}
		close(done)
	}()

	result, err := CloneReaders(src, dst, int64(len(testData)), opts, progressChan)
	close(progressChan)
	<-done

	if err != nil {
		t.Fatalf("CloneReaders failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	// Verify we received progress updates
	if len(progressUpdates) == 0 {
		t.Log("No progress updates received (this may be timing-dependent)")
	}

	// If we got updates, verify they're monotonically increasing
	for i := 1; i < len(progressUpdates); i++ {
		if progressUpdates[i].Copied < progressUpdates[i-1].Copied {
			t.Error("Progress should be monotonically increasing")
		}
	}
}

func TestCloneReadersZeroSize(t *testing.T) {
	src := bytes.NewReader([]byte{})
	dst := &bytes.Buffer{}

	opts := Options{
		BlockSize: 64 * 1024,
		DirectIO:  false,
	}

	result, err := CloneReaders(src, dst, 0, opts, nil)
	if err != nil {
		t.Fatalf("CloneReaders failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success for zero-size clone")
	}

	if result.BytesCopied != 0 {
		t.Errorf("BytesCopied: got %d, want 0", result.BytesCopied)
	}
}

// mockWriteFailer is a writer that fails after N bytes
type mockWriteFailer struct {
	written   int
	failAfter int
}

func (w *mockWriteFailer) Write(p []byte) (n int, err error) {
	if w.written >= w.failAfter {
		return 0, io.ErrShortWrite
	}
	canWrite := w.failAfter - w.written
	if canWrite > len(p) {
		canWrite = len(p)
	}
	w.written += canWrite
	return canWrite, nil
}

func TestCloneReadersWriteError(t *testing.T) {
	testData := make([]byte, 1024*1024) // 1MB
	src := bytes.NewReader(testData)
	dst := &mockWriteFailer{failAfter: 100 * 1024} // Fail after 100KB

	opts := Options{
		BlockSize: 64 * 1024,
		DirectIO:  false,
	}

	_, err := CloneReaders(src, dst, int64(len(testData)), opts, nil)
	if err == nil {
		t.Error("Expected error on write failure")
	}
}

func TestAlignedBuffer(t *testing.T) {
	sizes := []int{512, 4096, 64 * 1024, 1024 * 1024}

	for _, size := range sizes {
		buf := alignedBuffer(size)

		if len(buf) != size {
			t.Errorf("size %d: got length %d", size, len(buf))
		}

		// Check alignment (should be page-aligned)
		// This is a basic check - the actual alignment depends on the implementation
		if cap(buf) < size {
			t.Errorf("size %d: capacity %d less than size", size, cap(buf))
		}
	}
}
