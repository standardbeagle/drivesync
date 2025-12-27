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
	SourceSize       int64  // Source disk size in bytes
	DestSize         int64  // Destination disk size in bytes
	SourceLastUsed   int64  // Last used byte on source (from GPT)
	Difference       int64  // DestSize - SourceSize (negative if dest smaller)
	CanClone         bool   // Clone will fit on destination
	NeedsGPTFixup    bool   // GPT needs to be updated after clone
	CloneBytes       int64  // Number of bytes to actually clone
	UnallocatedTail  int64  // Unused space at end of destination
	HasEncryption    bool   // Source has encrypted partitions
	ErrorMessage     string // If CanClone is false, why
	ErrorDetails     string // Detailed instructions for fixing the error
}

// Analyze performs size analysis for a potential clone operation.
// hasEncryption indicates if the source has encrypted partitions (BitLocker, LUKS).
func Analyze(srcSize, dstSize, srcLastUsed int64) SizeAnalysis {
	return AnalyzeWithEncryption(srcSize, dstSize, srcLastUsed, false)
}

// AnalyzeWithEncryption performs size analysis with encryption awareness.
func AnalyzeWithEncryption(srcSize, dstSize, srcLastUsed int64, hasEncryption bool) SizeAnalysis {
	a := SizeAnalysis{
		SourceSize:     srcSize,
		DestSize:       dstSize,
		SourceLastUsed: srcLastUsed,
		Difference:     dstSize - srcSize,
		HasEncryption:  hasEncryption,
	}

	if dstSize >= srcSize {
		// Destination is same size or larger - always safe
		a.CanClone = true
		a.CloneBytes = srcSize
		a.NeedsGPTFixup = false
		a.UnallocatedTail = dstSize - srcSize
	} else if hasEncryption {
		// Destination smaller AND source has encryption - cannot safely truncate
		a.CanClone = false
		a.ErrorMessage = "Destination too small for encrypted source"
		a.ErrorDetails = `The source drive contains BitLocker or other encrypted volumes.
Encrypted volumes cannot be truncated - the entire disk must be cloned.

OPTIONS:
1. Use a larger destination drive (>= source size)
2. In Windows, shrink the main partition before cloning:
   - Open Settings > System > Storage > Disks & volumes
   - Select your main partition > Properties > Change size
   - Or: diskmgmt.msc > Right-click volume > Shrink Volume
3. Disable BitLocker, shrink partition, re-enable BitLocker`
	} else if srcLastUsed > 0 && dstSize >= srcLastUsed {
		// Destination is smaller but used data fits (no encryption)
		a.CanClone = true
		a.CloneBytes = srcLastUsed
		a.NeedsGPTFixup = true
		a.UnallocatedTail = dstSize - srcLastUsed
	} else if srcLastUsed > 0 && dstSize < srcLastUsed {
		// Destination too small for used data
		a.CanClone = false
		a.ErrorMessage = "Destination too small for source data"
		a.ErrorDetails = `The destination drive is smaller than the used space on source.

OPTIONS:
1. Use a larger destination drive
2. Delete unnecessary files from source
3. Shrink the source partition in Windows Disk Management`
	} else {
		// No GPT info, can't safely clone to smaller disk
		a.CanClone = false
		a.ErrorMessage = "Cannot clone to smaller destination"
		a.ErrorDetails = `The destination is smaller than the source and we cannot
determine how much space is actually used on the source.

OPTIONS:
1. Use a destination drive >= source size
2. This is the safest option for system drives`
	}

	return a
}
