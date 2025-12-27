//go:build windows

package devices

// BlkidInfo contains filesystem information.
// On Windows, this is populated via WMI instead of blkid.
type BlkidInfo struct {
	FSType       string
	Label        string
	UUID         string
	PartTypeGUID string // GPT partition type GUID
}

// GetBlkidInfo is a stub on Windows - WMI is used instead.
func GetBlkidInfo(devicePath string) BlkidInfo {
	return BlkidInfo{}
}

// PopulatePartitionInfo is a no-op on Windows.
// Partition info is populated during enumeration via WMI.
func PopulatePartitionInfo(disks []*Disk) {
	// Already populated by EnumerateWindows
}
