package clone

import (
	"testing"
	"time"
)

func TestProgressPercent(t *testing.T) {
	tests := []struct {
		name     string
		copied   int64
		total    int64
		expected float64
	}{
		{
			name:     "50% complete",
			copied:   500,
			total:    1000,
			expected: 50.0,
		},
		{
			name:     "0% complete",
			copied:   0,
			total:    1000,
			expected: 0.0,
		},
		{
			name:     "100% complete",
			copied:   1000,
			total:    1000,
			expected: 100.0,
		},
		{
			name:     "Zero total (edge case)",
			copied:   0,
			total:    0,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Progress{Copied: tt.copied, Total: tt.total}
			result := p.Percent()
			if result != tt.expected {
				t.Errorf("got %f, want %f", result, tt.expected)
			}
		})
	}
}

func TestProgressSpeedMBps(t *testing.T) {
	p := Progress{Speed: 100 * 1e6} // 100 MB/s
	result := p.SpeedMBps()
	if result != 100.0 {
		t.Errorf("got %f, want 100.0", result)
	}
}

func TestResultAvgSpeedMBps(t *testing.T) {
	r := Result{AvgSpeed: 500 * 1e6} // 500 MB/s
	result := r.AvgSpeedMBps()
	if result != 500.0 {
		t.Errorf("got %f, want 500.0", result)
	}
}

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name        string
		srcSize     int64
		dstSize     int64
		srcLastUsed int64
		wantCanClone bool
		wantNeedsGPT bool
	}{
		{
			name:        "Destination larger than source",
			srcSize:     1000,
			dstSize:     2000,
			srcLastUsed: 0,
			wantCanClone: true,
			wantNeedsGPT: false,
		},
		{
			name:        "Same size",
			srcSize:     1000,
			dstSize:     1000,
			srcLastUsed: 0,
			wantCanClone: true,
			wantNeedsGPT: false,
		},
		{
			name:        "Destination smaller but data fits",
			srcSize:     2000,
			dstSize:     1500,
			srcLastUsed: 1000,
			wantCanClone: true,
			wantNeedsGPT: true,
		},
		{
			name:        "Destination too small for data",
			srcSize:     2000,
			dstSize:     500,
			srcLastUsed: 1000,
			wantCanClone: false,
			wantNeedsGPT: false,
		},
		{
			name:        "Destination smaller, no GPT info",
			srcSize:     2000,
			dstSize:     1500,
			srcLastUsed: 0,
			wantCanClone: false,
			wantNeedsGPT: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Analyze(tt.srcSize, tt.dstSize, tt.srcLastUsed)

			if result.CanClone != tt.wantCanClone {
				t.Errorf("CanClone: got %v, want %v", result.CanClone, tt.wantCanClone)
			}

			if result.NeedsGPTFixup != tt.wantNeedsGPT {
				t.Errorf("NeedsGPTFixup: got %v, want %v", result.NeedsGPTFixup, tt.wantNeedsGPT)
			}

			// Verify other fields
			if result.SourceSize != tt.srcSize {
				t.Errorf("SourceSize: got %d, want %d", result.SourceSize, tt.srcSize)
			}

			if result.DestSize != tt.dstSize {
				t.Errorf("DestSize: got %d, want %d", result.DestSize, tt.dstSize)
			}

			if result.Difference != tt.dstSize-tt.srcSize {
				t.Errorf("Difference: got %d, want %d", result.Difference, tt.dstSize-tt.srcSize)
			}
		})
	}
}

func TestAnalyze2TBDrives(t *testing.T) {
	// Real-world test case: 2TB drives with different actual sizes
	wdBlue := int64(2000398934016)      // WD Blue 2TB
	samsung := int64(1999844147200)     // Samsung 870 2TB
	usedSpace := int64(500 * 1e9)       // 500GB used

	result := Analyze(wdBlue, samsung, usedSpace)

	if !result.CanClone {
		t.Error("Should be able to clone 500GB to a 2TB drive")
	}

	if !result.NeedsGPTFixup {
		t.Error("Should need GPT fixup when destination is smaller")
	}

	expectedDiff := samsung - wdBlue // Should be negative
	if result.Difference != expectedDiff {
		t.Errorf("Difference: got %d, want %d", result.Difference, expectedDiff)
	}

	// About 554MB difference
	diffMB := float64(-result.Difference) / 1e6
	if diffMB < 500 || diffMB > 600 {
		t.Errorf("Difference should be ~554MB, got %.0fMB", diffMB)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.BlockSize != 64*1024 {
		t.Errorf("BlockSize: got %d, want %d", opts.BlockSize, 64*1024)
	}

	if !opts.DirectIO {
		t.Error("DirectIO should be true by default")
	}

	if opts.Verify {
		t.Error("Verify should be false by default")
	}

	if !opts.FixupGPT {
		t.Error("FixupGPT should be true by default")
	}
}

func TestProgressRemainingTime(t *testing.T) {
	p := Progress{
		Copied:   500 * 1e9, // 500GB
		Total:    1000 * 1e9, // 1TB
		AvgSpeed: 500 * 1e6, // 500 MB/s
		Elapsed:  1000 * time.Second,
	}

	// Remaining should be about 1000 seconds at 500 MB/s for 500GB
	// This test just verifies the fields are set correctly
	if p.Copied != 500*1e9 {
		t.Errorf("Copied: got %d", p.Copied)
	}

	if p.AvgSpeedMBps() != 500 {
		t.Errorf("AvgSpeedMBps: got %f", p.AvgSpeedMBps())
	}
}
