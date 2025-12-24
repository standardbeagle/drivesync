package gpt

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/beagle/drivesync/internal/devices"
)

func TestGUIDString(t *testing.T) {
	tests := []struct {
		name     string
		guid     GUID
		expected string
	}{
		{
			name:     "EFI System Partition GUID",
			guid:     mustParseGUID("C12A7328-F81F-11D2-BA4B-00A0C93EC93B"),
			expected: "C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
		},
		{
			name:     "Zero GUID",
			guid:     GUID{},
			expected: "00000000-0000-0000-0000-000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.guid.String()
			if result != tt.expected {
				t.Errorf("got %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGUIDIsZero(t *testing.T) {
	tests := []struct {
		name     string
		guid     GUID
		expected bool
	}{
		{
			name:     "Zero GUID",
			guid:     GUID{},
			expected: true,
		},
		{
			name:     "Non-zero GUID",
			guid:     mustParseGUID("C12A7328-F81F-11D2-BA4B-00A0C93EC93B"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.guid.IsZero()
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseGUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "Valid GUID",
			input:   "C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
			wantErr: false,
		},
		{
			name:    "Invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "Empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseGUID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGUID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPartitionEntryGetSetName(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Simple name",
			input: "EFI System",
		},
		{
			name:  "Windows name",
			input: "Windows 11",
		},
		{
			name:  "Empty name",
			input: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p PartitionEntry
			p.SetPartitionName(tt.input)
			result := p.GetPartitionName()
			if result != tt.input {
				t.Errorf("got %q, want %q", result, tt.input)
			}
		})
	}
}

func TestPartitionEntrySizeInSectors(t *testing.T) {
	tests := []struct {
		name     string
		firstLBA uint64
		lastLBA  uint64
		expected uint64
	}{
		{
			name:     "Normal partition",
			firstLBA: 2048,
			lastLBA:  206847,
			expected: 204800,
		},
		{
			name:     "Single sector",
			firstLBA: 100,
			lastLBA:  100,
			expected: 1,
		},
		{
			name:     "Invalid (last < first)",
			firstLBA: 100,
			lastLBA:  50,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PartitionEntry{
				FirstLBA: tt.firstLBA,
				LastLBA:  tt.lastLBA,
			}
			result := p.SizeInSectors()
			if result != tt.expected {
				t.Errorf("got %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestCalculateHeaderCRC32(t *testing.T) {
	// Create a minimal valid header
	h := &Header{
		Signature:          Signature,
		Revision:           0x00010000,
		HeaderSize:         92,
		MyLBA:              1,
		BackupLBA:          999999,
		FirstUsableLBA:     34,
		LastUsableLBA:      999966,
		PartitionEntryLBA:  2,
		NumPartitions:      128,
		PartitionEntrySize: 128,
	}

	// Calculate CRC twice - should be consistent
	crc1 := CalculateHeaderCRC32(h)
	crc2 := CalculateHeaderCRC32(h)

	if crc1 != crc2 {
		t.Errorf("CRC32 not consistent: %x != %x", crc1, crc2)
	}

	// Modify header, CRC should change
	h.BackupLBA = 999998
	crc3 := CalculateHeaderCRC32(h)

	if crc1 == crc3 {
		t.Error("CRC32 should change when header changes")
	}
}

func TestReadTableFromReader(t *testing.T) {
	// Create a mock GPT disk image
	img := createMockGPTImage(t)

	table, err := ReadTableFromReader(bytes.NewReader(img))
	if err != nil {
		t.Fatalf("ReadTableFromReader failed: %v", err)
	}

	if table.Header.Signature != Signature {
		t.Errorf("wrong signature: got %x, want %x", table.Header.Signature, Signature)
	}

	validParts := table.ValidPartitions()
	if len(validParts) != 2 {
		t.Errorf("expected 2 valid partitions, got %d", len(validParts))
	}
}

func TestTableLastUsedLBA(t *testing.T) {
	table := &Table{
		Partitions: []PartitionEntry{
			{TypeGUID: mustParseGUID(devices.GUIDEFISystem), FirstLBA: 2048, LastLBA: 206847},
			{TypeGUID: mustParseGUID(devices.GUIDBasicData), FirstLBA: 206848, LastLBA: 1000000},
			{TypeGUID: GUID{}, FirstLBA: 0, LastLBA: 0}, // Empty entry
		},
	}

	lastLBA := table.LastUsedLBA()
	if lastLBA != 1000000 {
		t.Errorf("got %d, want 1000000", lastLBA)
	}
}

// Helper functions

func mustParseGUID(s string) GUID {
	g, err := ParseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

func createMockGPTImage(t *testing.T) []byte {
	t.Helper()

	// Create a 1MB image
	img := make([]byte, 1024*1024)

	// Protective MBR (sector 0) - just leave empty for now

	// GPT Header (sector 1, offset 512)
	header := Header{
		Signature:          Signature,
		Revision:           0x00010000,
		HeaderSize:         92,
		MyLBA:              1,
		BackupLBA:          2047, // Last sector of 1MB image
		FirstUsableLBA:     34,
		LastUsableLBA:      2014,
		PartitionEntryLBA:  2,
		NumPartitions:      128,
		PartitionEntrySize: 128,
	}

	// Create two partitions
	partitions := make([]PartitionEntry, 128)

	// EFI System Partition
	partitions[0] = PartitionEntry{
		TypeGUID: mustParseGUID(devices.GUIDEFISystem),
		PartGUID: mustParseGUID("A1B2C3D4-E5F6-7890-1234-567890ABCDEF"),
		FirstLBA: 34,
		LastLBA:  2047,
	}
	partitions[0].SetPartitionName("EFI System")

	// Basic Data Partition
	partitions[1] = PartitionEntry{
		TypeGUID: mustParseGUID(devices.GUIDBasicData),
		PartGUID: mustParseGUID("B2C3D4E5-F678-9012-3456-7890ABCDEF01"),
		FirstLBA: 2048,
		LastLBA:  1999,
	}
	partitions[1].SetPartitionName("Windows")

	// Calculate partition entries CRC
	header.PartitionEntryCRC32 = CalculatePartitionEntriesCRC32(partitions, 128, 128)
	header.HeaderCRC32 = CalculateHeaderCRC32(&header)

	// Write header to image
	writeHeaderToBytes(img[512:], &header)

	// Write partition entries to image (starting at sector 2)
	for i, p := range partitions {
		offset := 1024 + i*128
		writePartitionEntryToBytes(img[offset:], &p)
	}

	return img
}

func writeHeaderToBytes(buf []byte, h *Header) {
	binary.LittleEndian.PutUint64(buf[0:8], h.Signature)
	binary.LittleEndian.PutUint32(buf[8:12], h.Revision)
	binary.LittleEndian.PutUint32(buf[12:16], h.HeaderSize)
	binary.LittleEndian.PutUint32(buf[16:20], h.HeaderCRC32)
	binary.LittleEndian.PutUint32(buf[20:24], h.Reserved)
	binary.LittleEndian.PutUint64(buf[24:32], h.MyLBA)
	binary.LittleEndian.PutUint64(buf[32:40], h.BackupLBA)
	binary.LittleEndian.PutUint64(buf[40:48], h.FirstUsableLBA)
	binary.LittleEndian.PutUint64(buf[48:56], h.LastUsableLBA)
	copy(buf[56:72], h.DiskGUID[:])
	binary.LittleEndian.PutUint64(buf[72:80], h.PartitionEntryLBA)
	binary.LittleEndian.PutUint32(buf[80:84], h.NumPartitions)
	binary.LittleEndian.PutUint32(buf[84:88], h.PartitionEntrySize)
	binary.LittleEndian.PutUint32(buf[88:92], h.PartitionEntryCRC32)
}

func writePartitionEntryToBytes(buf []byte, p *PartitionEntry) {
	copy(buf[0:16], p.TypeGUID[:])
	copy(buf[16:32], p.PartGUID[:])
	binary.LittleEndian.PutUint64(buf[32:40], p.FirstLBA)
	binary.LittleEndian.PutUint64(buf[40:48], p.LastLBA)
	binary.LittleEndian.PutUint64(buf[48:56], p.Attributes)
	copy(buf[56:128], p.Name[:])
}
