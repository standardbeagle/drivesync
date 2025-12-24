// Package gpt provides GPT (GUID Partition Table) parsing and writing.
package gpt

import (
	"encoding/binary"
	"fmt"
)

const (
	// Signature is the GPT header signature "EFI PART".
	Signature = 0x5452415020494645

	// HeaderSize is the size of the GPT header in bytes.
	HeaderSize = 92

	// PartitionEntrySize is the standard size of a partition entry.
	PartitionEntrySize = 128

	// MaxPartitions is the typical maximum number of partitions.
	MaxPartitions = 128

	// SectorSize is the default sector size.
	SectorSize = 512
)

// Header represents a GPT header (located at LBA 1 and backup at last LBA).
type Header struct {
	Signature         uint64   // "EFI PART" = 0x5452415020494645
	Revision          uint32   // Usually 0x00010000 (1.0)
	HeaderSize        uint32   // Size of header (usually 92)
	HeaderCRC32       uint32   // CRC32 of header (with this field zeroed)
	Reserved          uint32   // Must be zero
	MyLBA             uint64   // LBA of this header
	BackupLBA         uint64   // LBA of backup header
	FirstUsableLBA    uint64   // First usable LBA for partitions
	LastUsableLBA     uint64   // Last usable LBA for partitions
	DiskGUID          GUID     // Unique disk identifier
	PartitionEntryLBA uint64   // Starting LBA of partition entries
	NumPartitions     uint32   // Number of partition entries
	PartitionEntrySize uint32  // Size of each partition entry (usually 128)
	PartitionEntryCRC32 uint32 // CRC32 of partition entries
}

// PartitionEntry represents a single GPT partition entry.
type PartitionEntry struct {
	TypeGUID   GUID     // Partition type GUID
	PartGUID   GUID     // Unique partition GUID
	FirstLBA   uint64   // First LBA of partition
	LastLBA    uint64   // Last LBA of partition (inclusive)
	Attributes uint64   // Partition attributes
	Name       [72]byte // Partition name (UTF-16LE, null-terminated)
}

// GUID represents a 128-bit globally unique identifier.
type GUID [16]byte

// String returns the GUID in standard format (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx).
func (g GUID) String() string {
	return fmt.Sprintf("%08X-%04X-%04X-%02X%02X-%012X",
		binary.LittleEndian.Uint32(g[0:4]),
		binary.LittleEndian.Uint16(g[4:6]),
		binary.LittleEndian.Uint16(g[6:8]),
		g[8], g[9],
		g[10:16])
}

// IsZero returns true if the GUID is all zeros.
func (g GUID) IsZero() bool {
	for _, b := range g {
		if b != 0 {
			return false
		}
	}
	return true
}

// ParseGUID parses a GUID string into a GUID.
func ParseGUID(s string) (GUID, error) {
	var g GUID
	var d1 uint32
	var d2, d3 uint16
	var d4, d5 byte
	var d6 [6]byte

	n, err := fmt.Sscanf(s, "%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		&d1, &d2, &d3, &d4, &d5, &d6[0], &d6[1], &d6[2], &d6[3], &d6[4], &d6[5])
	if err != nil || n != 11 {
		return g, fmt.Errorf("invalid GUID format: %s", s)
	}

	binary.LittleEndian.PutUint32(g[0:4], d1)
	binary.LittleEndian.PutUint16(g[4:6], d2)
	binary.LittleEndian.PutUint16(g[6:8], d3)
	g[8] = d4
	g[9] = d5
	copy(g[10:16], d6[:])

	return g, nil
}

// GetPartitionName returns the partition name as a Go string.
func (p *PartitionEntry) GetPartitionName() string {
	// Name is UTF-16LE, null-terminated
	name := make([]rune, 0, 36)
	for i := 0; i < len(p.Name); i += 2 {
		c := binary.LittleEndian.Uint16(p.Name[i : i+2])
		if c == 0 {
			break
		}
		name = append(name, rune(c))
	}
	return string(name)
}

// SetPartitionName sets the partition name from a Go string.
func (p *PartitionEntry) SetPartitionName(name string) {
	// Clear the name field
	for i := range p.Name {
		p.Name[i] = 0
	}

	// Convert to UTF-16LE
	runes := []rune(name)
	for i, r := range runes {
		if i*2+1 >= len(p.Name) {
			break
		}
		binary.LittleEndian.PutUint16(p.Name[i*2:], uint16(r))
	}
}

// SizeInSectors returns the partition size in sectors.
func (p *PartitionEntry) SizeInSectors() uint64 {
	if p.LastLBA >= p.FirstLBA {
		return p.LastLBA - p.FirstLBA + 1
	}
	return 0
}

// SizeInBytes returns the partition size in bytes (assuming 512-byte sectors).
func (p *PartitionEntry) SizeInBytes() uint64 {
	return p.SizeInSectors() * SectorSize
}
