package gpt

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// Table represents a complete GPT (header + partitions).
type Table struct {
	Header     Header
	Partitions []PartitionEntry
	SectorSize int // Sector size in bytes (default 512)
}

// ReadTable reads the GPT from a device or file.
func ReadTable(path string) (*Table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open device: %w", err)
	}
	defer f.Close()

	return ReadTableFromReader(f)
}

// ReadTableFromReader reads the GPT from an io.ReadSeeker.
func ReadTableFromReader(r io.ReadSeeker) (*Table, error) {
	// Read primary GPT header at LBA 1 (offset 512)
	if _, err := r.Seek(SectorSize, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to GPT header: %w", err)
	}

	header, err := readHeader(r)
	if err != nil {
		return nil, fmt.Errorf("read GPT header: %w", err)
	}

	// Verify signature
	if header.Signature != Signature {
		return nil, fmt.Errorf("invalid GPT signature: %x (expected %x)", header.Signature, Signature)
	}

	// Read partition entries
	partitionOffset := int64(header.PartitionEntryLBA) * SectorSize
	if _, err := r.Seek(partitionOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to partition entries: %w", err)
	}

	partitions := make([]PartitionEntry, header.NumPartitions)
	for i := uint32(0); i < header.NumPartitions; i++ {
		entry, err := readPartitionEntry(r, header.PartitionEntrySize)
		if err != nil {
			return nil, fmt.Errorf("read partition entry %d: %w", i, err)
		}
		partitions[i] = entry
	}

	return &Table{
		Header:     header,
		Partitions: partitions,
		SectorSize: SectorSize,
	}, nil
}

// readHeader reads a GPT header from the current position.
func readHeader(r io.Reader) (Header, error) {
	var h Header
	var buf [HeaderSize]byte

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return h, err
	}

	h.Signature = binary.LittleEndian.Uint64(buf[0:8])
	h.Revision = binary.LittleEndian.Uint32(buf[8:12])
	h.HeaderSize = binary.LittleEndian.Uint32(buf[12:16])
	h.HeaderCRC32 = binary.LittleEndian.Uint32(buf[16:20])
	h.Reserved = binary.LittleEndian.Uint32(buf[20:24])
	h.MyLBA = binary.LittleEndian.Uint64(buf[24:32])
	h.BackupLBA = binary.LittleEndian.Uint64(buf[32:40])
	h.FirstUsableLBA = binary.LittleEndian.Uint64(buf[40:48])
	h.LastUsableLBA = binary.LittleEndian.Uint64(buf[48:56])
	copy(h.DiskGUID[:], buf[56:72])
	h.PartitionEntryLBA = binary.LittleEndian.Uint64(buf[72:80])
	h.NumPartitions = binary.LittleEndian.Uint32(buf[80:84])
	h.PartitionEntrySize = binary.LittleEndian.Uint32(buf[84:88])
	h.PartitionEntryCRC32 = binary.LittleEndian.Uint32(buf[88:92])

	return h, nil
}

// readPartitionEntry reads a single partition entry.
func readPartitionEntry(r io.Reader, entrySize uint32) (PartitionEntry, error) {
	var p PartitionEntry

	buf := make([]byte, entrySize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return p, err
	}

	copy(p.TypeGUID[:], buf[0:16])
	copy(p.PartGUID[:], buf[16:32])
	p.FirstLBA = binary.LittleEndian.Uint64(buf[32:40])
	p.LastLBA = binary.LittleEndian.Uint64(buf[40:48])
	p.Attributes = binary.LittleEndian.Uint64(buf[48:56])
	copy(p.Name[:], buf[56:128])

	return p, nil
}

// ValidPartitions returns only non-empty partitions.
func (t *Table) ValidPartitions() []PartitionEntry {
	var valid []PartitionEntry
	for _, p := range t.Partitions {
		if !p.TypeGUID.IsZero() {
			valid = append(valid, p)
		}
	}
	return valid
}

// LastUsedLBA returns the LBA of the last used sector across all partitions.
func (t *Table) LastUsedLBA() uint64 {
	var lastLBA uint64
	for _, p := range t.ValidPartitions() {
		if p.LastLBA > lastLBA {
			lastLBA = p.LastLBA
		}
	}
	return lastLBA
}

// LastUsedByte returns the byte offset of the last used byte.
func (t *Table) LastUsedByte() int64 {
	return int64(t.LastUsedLBA()+1) * int64(t.SectorSize)
}

// CalculateHeaderCRC32 computes the CRC32 for a header.
func CalculateHeaderCRC32(h *Header) uint32 {
	var buf [HeaderSize]byte

	binary.LittleEndian.PutUint64(buf[0:8], h.Signature)
	binary.LittleEndian.PutUint32(buf[8:12], h.Revision)
	binary.LittleEndian.PutUint32(buf[12:16], h.HeaderSize)
	// CRC32 field is zeroed for calculation
	binary.LittleEndian.PutUint32(buf[16:20], 0)
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

	return crc32.ChecksumIEEE(buf[:h.HeaderSize])
}

// CalculatePartitionEntriesCRC32 computes the CRC32 for partition entries.
func CalculatePartitionEntriesCRC32(partitions []PartitionEntry, entrySize uint32, numEntries uint32) uint32 {
	buf := make([]byte, int(entrySize)*int(numEntries))

	for i, p := range partitions {
		if uint32(i) >= numEntries {
			break
		}
		offset := i * int(entrySize)
		copy(buf[offset:offset+16], p.TypeGUID[:])
		copy(buf[offset+16:offset+32], p.PartGUID[:])
		binary.LittleEndian.PutUint64(buf[offset+32:offset+40], p.FirstLBA)
		binary.LittleEndian.PutUint64(buf[offset+40:offset+48], p.LastLBA)
		binary.LittleEndian.PutUint64(buf[offset+48:offset+56], p.Attributes)
		copy(buf[offset+56:offset+128], p.Name[:])
	}

	return crc32.ChecksumIEEE(buf)
}

// VerifyHeader verifies the header CRC32.
func (t *Table) VerifyHeader() error {
	expected := CalculateHeaderCRC32(&t.Header)
	if t.Header.HeaderCRC32 != expected {
		return fmt.Errorf("header CRC32 mismatch: got %x, expected %x", t.Header.HeaderCRC32, expected)
	}
	return nil
}

// VerifyPartitions verifies the partition entries CRC32.
func (t *Table) VerifyPartitions() error {
	expected := CalculatePartitionEntriesCRC32(t.Partitions, t.Header.PartitionEntrySize, t.Header.NumPartitions)
	if t.Header.PartitionEntryCRC32 != expected {
		return fmt.Errorf("partition entries CRC32 mismatch: got %x, expected %x", t.Header.PartitionEntryCRC32, expected)
	}
	return nil
}
