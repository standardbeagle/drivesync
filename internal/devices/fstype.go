package devices

import "strings"

// NormalizeFSType converts filesystem type to a display-friendly format.
func NormalizeFSType(fsType string) string {
	switch strings.ToLower(fsType) {
	case "vfat", "fat32", "fat16", "fat12", "msdos":
		return "FAT32"
	case "ntfs", "ntfs-3g":
		return "NTFS"
	case "bitlocker":
		return "BitLocker"
	case "crypto_luks", "luks":
		return "LUKS"
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

// IsEncrypted returns true if the filesystem type indicates encryption.
func IsEncrypted(fsType string) bool {
	switch strings.ToLower(fsType) {
	case "bitlocker", "crypto_luks", "luks":
		return true
	default:
		return false
	}
}

// HasEncryptedPartitions checks if any partition on the disk is encrypted.
func HasEncryptedPartitions(disk *Disk) bool {
	for _, part := range disk.Partitions {
		if IsEncrypted(part.FSType) {
			return true
		}
	}
	return false
}

// GetEncryptedPartitions returns a list of encrypted partitions on the disk.
func GetEncryptedPartitions(disk *Disk) []*Partition {
	var encrypted []*Partition
	for _, part := range disk.Partitions {
		if IsEncrypted(part.FSType) {
			encrypted = append(encrypted, part)
		}
	}
	return encrypted
}
