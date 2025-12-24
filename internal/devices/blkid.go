package devices

import (
	"bytes"
	"os/exec"
	"strings"
)

// BlkidInfo contains filesystem information from blkid.
type BlkidInfo struct {
	FSType string
	Label  string
	UUID   string
}

// GetBlkidInfo runs blkid on a device and parses the output.
// Returns empty BlkidInfo if blkid fails or isn't available.
func GetBlkidInfo(devicePath string) BlkidInfo {
	info := BlkidInfo{}

	// Try running blkid
	cmd := exec.Command("blkid", "-o", "export", devicePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return info
	}

	// Parse output (KEY=VALUE format, one per line)
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "TYPE":
			info.FSType = value
		case "LABEL":
			info.Label = value
		case "UUID":
			info.UUID = value
		}
	}

	return info
}

// PopulatePartitionInfo fills in filesystem details for partitions.
// This runs blkid for each partition to get filesystem type, label, and UUID.
func PopulatePartitionInfo(disks []*Disk) {
	for _, disk := range disks {
		for _, part := range disk.Partitions {
			info := GetBlkidInfo(part.Path)
			part.FSType = info.FSType
			part.FSLabel = info.Label
			part.FSUUID = info.UUID
		}
	}
}

// NormalizeFSType converts filesystem type to a display-friendly format.
func NormalizeFSType(fsType string) string {
	switch strings.ToLower(fsType) {
	case "vfat", "fat32", "fat16", "fat12", "msdos":
		return "FAT32"
	case "ntfs", "ntfs-3g":
		return "NTFS"
	case "ext4":
		return "ext4"
	case "ext3":
		return "ext3"
	case "ext2":
		return "ext2"
	case "btrfs":
		return "Btrfs"
	case "xfs":
		return "XFS"
	case "swap":
		return "Swap"
	case "":
		return ""
	default:
		return fsType
	}
}
