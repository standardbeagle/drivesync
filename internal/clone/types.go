// Package clone provides block-level disk cloning functionality.
package clone

import (
	"time"
)

// Options configures the clone operation.
type Options struct {
	BlockSize   int   // Block size in bytes (default 64KB)
	Verify      bool  // Read-back verification after clone
	DirectIO    bool  // Use O_DIRECT to bypass page cache
	CloneBytes  int64 // Number of bytes to clone (0 = entire source)
	FixupGPT    bool  // Update GPT backup after clone
	DestSize    int64 // Destination size for GPT fixup
}

// DefaultOptions returns sensible defaults for cloning.
func DefaultOptions() Options {
	return Options{
		BlockSize: 64 * 1024, // 64KB
		DirectIO:  true,
		Verify:    false,
		FixupGPT:  true,
	}
}

// Progress represents the current state of a clone operation.
type Progress struct {
	Copied      int64         // Bytes copied so far
	Total       int64         // Total bytes to copy
	Speed       float64       // Current speed in bytes/second
	AvgSpeed    float64       // Average speed in bytes/second
	Elapsed     time.Duration // Time elapsed
	Remaining   time.Duration // Estimated time remaining
	Verifying   bool          // Currently in verification phase
	VerifyBytes int64         // Bytes verified (if verifying)
}

// Percent returns the completion percentage (0-100).
func (p Progress) Percent() float64 {
	if p.Total == 0 {
		return 0
	}
	return float64(p.Copied) / float64(p.Total) * 100
}

// SpeedMBps returns the current speed in MB/s.
func (p Progress) SpeedMBps() float64 {
	return p.Speed / 1e6
}

// AvgSpeedMBps returns the average speed in MB/s.
func (p Progress) AvgSpeedMBps() float64 {
	return p.AvgSpeed / 1e6
}

// Result contains the outcome of a clone operation.
type Result struct {
	Success      bool          // Clone completed successfully
	Error        error         // Error if not successful
	BytesCopied  int64         // Total bytes copied
	Duration     time.Duration // Total time taken
	AvgSpeed     float64       // Average speed in bytes/second
	Verified     bool          // Verification was performed
	VerifyPassed bool          // Verification passed (if performed)
}

// AvgSpeedMBps returns the average speed in MB/s.
func (r Result) AvgSpeedMBps() float64 {
	return r.AvgSpeed / 1e6
}

// SizeAnalysis represents the analysis of source and destination sizes.
type SizeAnalysis struct {
	SourceSize      int64 // Source disk size in bytes
	DestSize        int64 // Destination disk size in bytes
	SourceLastUsed  int64 // Last used byte on source (from GPT)
	Difference      int64 // DestSize - SourceSize (negative if dest smaller)
	CanClone        bool  // Clone will fit on destination
	NeedsGPTFixup   bool  // GPT needs to be updated after clone
	CloneBytes      int64 // Number of bytes to actually clone
	UnallocatedTail int64 // Unused space at end of destination
	ErrorMessage    string // If CanClone is false, why
}

// Analyze performs size analysis for a potential clone operation.
func Analyze(srcSize, dstSize, srcLastUsed int64) SizeAnalysis {
	a := SizeAnalysis{
		SourceSize:     srcSize,
		DestSize:       dstSize,
		SourceLastUsed: srcLastUsed,
		Difference:     dstSize - srcSize,
	}

	if dstSize >= srcSize {
		// Destination is same size or larger - straightforward clone
		a.CanClone = true
		a.CloneBytes = srcSize
		a.NeedsGPTFixup = false
		a.UnallocatedTail = dstSize - srcSize
	} else if srcLastUsed > 0 && dstSize >= srcLastUsed {
		// Destination is smaller but used data fits
		a.CanClone = true
		a.CloneBytes = srcLastUsed
		a.NeedsGPTFixup = true
		a.UnallocatedTail = dstSize - srcLastUsed
	} else if srcLastUsed > 0 && dstSize < srcLastUsed {
		// Destination too small for used data
		a.CanClone = false
		a.ErrorMessage = "destination too small for source data; shrink source partition first"
	} else {
		// No GPT info, can't safely clone to smaller disk
		a.CanClone = false
		a.ErrorMessage = "destination smaller than source and cannot determine used space"
	}

	return a
}
