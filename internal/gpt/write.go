package gpt

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// WriteTable writes the GPT to a device or file.
// This writes both the primary and backup GPT headers.
func WriteTable(path string, t *Table) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	defer f.Close()

	return WriteTableToWriter(f, t)
}

// WriteTableToWriter writes the GPT to an io.WriteSeeker.
func WriteTableToWriter(w io.WriteSeeker, t *Table) error {
	sectorSize := int64(t.SectorSize)
	if sectorSize == 0 {
		sectorSize = SectorSize
	}

	// Update partition entries CRC
	t.Header.PartitionEntryCRC32 = CalculatePartitionEntriesCRC32(
		t.Partitions,
		t.Header.PartitionEntrySize,
		t.Header.NumPartitions,
	)

	// Write primary header at LBA 1
	primaryHeader := t.Header
	primaryHeader.MyLBA = 1
	primaryHeader.PartitionEntryLBA = 2 // Partition entries start at LBA 2
	primaryHeader.HeaderCRC32 = CalculateHeaderCRC32(&primaryHeader)

	if _, err := w.Seek(sectorSize, io.SeekStart); err != nil {
		return fmt.Errorf("seek to primary header: %w", err)
	}
	if err := writeHeader(w, &primaryHeader); err != nil {
		return fmt.Errorf("write primary header: %w", err)
	}

	// Write partition entries after primary header (LBA 2)
	if _, err := w.Seek(2*sectorSize, io.SeekStart); err != nil {
		return fmt.Errorf("seek to primary partition entries: %w", err)
	}
	if err := writePartitionEntries(w, t.Partitions, t.Header.PartitionEntrySize, t.Header.NumPartitions); err != nil {
		return fmt.Errorf("write primary partition entries: %w", err)
	}

	// Write backup partition entries before backup header
	backupPartitionLBA := t.Header.BackupLBA - 32 // 32 sectors for partition entries
	if _, err := w.Seek(int64(backupPartitionLBA)*sectorSize, io.SeekStart); err != nil {
		return fmt.Errorf("seek to backup partition entries: %w", err)
	}
	if err := writePartitionEntries(w, t.Partitions, t.Header.PartitionEntrySize, t.Header.NumPartitions); err != nil {
		return fmt.Errorf("write backup partition entries: %w", err)
	}

	// Write backup header at last LBA
	backupHeader := t.Header
	backupHeader.MyLBA = t.Header.BackupLBA
	backupHeader.BackupLBA = 1
	backupHeader.PartitionEntryLBA = backupPartitionLBA
	backupHeader.HeaderCRC32 = CalculateHeaderCRC32(&backupHeader)

	if _, err := w.Seek(int64(t.Header.BackupLBA)*sectorSize, io.SeekStart); err != nil {
		return fmt.Errorf("seek to backup header: %w", err)
	}
	if err := writeHeader(w, &backupHeader); err != nil {
		return fmt.Errorf("write backup header: %w", err)
	}

	return nil
}

// writeHeader writes a GPT header at the current position.
func writeHeader(w io.Writer, h *Header) error {
	var buf [HeaderSize]byte

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

	_, err := w.Write(buf[:])
	return err
}

// writePartitionEntries writes partition entries at the current position.
func writePartitionEntries(w io.Writer, partitions []PartitionEntry, entrySize, numEntries uint32) error {
	buf := make([]byte, entrySize)

	for i := uint32(0); i < numEntries; i++ {
		// Clear buffer
		for j := range buf {
			buf[j] = 0
		}

		if int(i) < len(partitions) {
			p := partitions[i]
			copy(buf[0:16], p.TypeGUID[:])
			copy(buf[16:32], p.PartGUID[:])
			binary.LittleEndian.PutUint64(buf[32:40], p.FirstLBA)
			binary.LittleEndian.PutUint64(buf[40:48], p.LastLBA)
			binary.LittleEndian.PutUint64(buf[48:56], p.Attributes)
			copy(buf[56:128], p.Name[:])
		}

		if _, err := w.Write(buf); err != nil {
			return err
		}
	}

	return nil
}

// FixupGPT updates the GPT to match a new disk size.
// This is used when cloning to a slightly smaller disk.
func FixupGPT(devicePath string, newSizeBytes int64) error {
	// Read existing GPT
	table, err := ReadTable(devicePath)
	if err != nil {
		return fmt.Errorf("read GPT: %w", err)
	}

	sectorSize := int64(table.SectorSize)
	if sectorSize == 0 {
		sectorSize = SectorSize
	}

	newLastLBA := uint64((newSizeBytes / sectorSize) - 1)

	// Update header for new size
	table.Header.BackupLBA = newLastLBA
	table.Header.LastUsableLBA = newLastLBA - 33 // 33 sectors for backup GPT

	// Write updated GPT
	return WriteTable(devicePath, table)
}

// FixupGPTWriter updates the GPT to match a new disk size using an io.ReadWriteSeeker.
func FixupGPTWriter(rws io.ReadWriteSeeker, newSizeBytes int64, sectorSize int64) error {
	if sectorSize == 0 {
		sectorSize = SectorSize
	}

	// Read existing GPT header
	if _, err := rws.Seek(sectorSize, io.SeekStart); err != nil {
		return fmt.Errorf("seek to GPT header: %w", err)
	}

	header, err := readHeader(rws)
	if err != nil {
		return fmt.Errorf("read GPT header: %w", err)
	}

	// Read partition entries
	partitionOffset := int64(header.PartitionEntryLBA) * sectorSize
	if _, err := rws.Seek(partitionOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seek to partition entries: %w", err)
	}

	partitions := make([]PartitionEntry, header.NumPartitions)
	for i := uint32(0); i < header.NumPartitions; i++ {
		entry, err := readPartitionEntry(rws, header.PartitionEntrySize)
		if err != nil {
			return fmt.Errorf("read partition entry %d: %w", i, err)
		}
		partitions[i] = entry
	}

	newLastLBA := uint64((newSizeBytes / sectorSize) - 1)

	// Update header for new size
	header.BackupLBA = newLastLBA
	header.LastUsableLBA = newLastLBA - 33

	table := &Table{
		Header:     header,
		Partitions: partitions,
		SectorSize: int(sectorSize),
	}

	return WriteTableToWriter(rws, table)
}
