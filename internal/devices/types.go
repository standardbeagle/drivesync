// Package devices provides drive enumeration and device information parsing.
package devices

// Transport represents the drive connection type.
type Transport string

const (
	TransportNVMe    Transport = "NVMe"
	TransportUSB     Transport = "USB"
	TransportSATA    Transport = "SATA"
	TransportMMC     Transport = "SD/eMMC"
	TransportUnknown Transport = "Unknown"
)

// DriveType indicates whether the drive is an SSD or HDD.
type DriveType string

const (
	DriveTypeSSD     DriveType = "SSD"
	DriveTypeHDD     DriveType = "HDD"
	DriveTypeUnknown DriveType = "Unknown"
)

// Disk represents a physical block device.
type Disk struct {
	Path        string       // /dev/sda, /dev/nvme0n1
	Name        string       // sda, nvme0n1
	Model       string       // Drive model name
	Vendor      string       // Drive vendor
	Serial      string       // Serial number
	SizeBytes   int64        // Total size in bytes
	SizeSectors int64        // Total size in 512-byte sectors
	SectorSize  int          // Logical sector size (usually 512 or 4096)
	Transport   Transport    // Connection type
	DriveType   DriveType    // SSD or HDD
	Removable   bool         // Is removable media
	IsUSB       bool         // Connected via USB
	Partitions  []*Partition // Partition list
	SysfsPath   string       // Full sysfs path for debugging
}

// SizeGB returns the drive size in gigabytes (base 10, as drive manufacturers use).
func (d *Disk) SizeGB() float64 {
	return float64(d.SizeBytes) / 1e9
}

// DisplayName returns a human-readable name for the drive.
func (d *Disk) DisplayName() string {
	name := d.Model
	if name == "" {
		name = d.Vendor
	}
	if name == "" {
		name = d.Name
	}
	if d.IsUSB {
		name += " (USB)"
	}
	return name
}

// LastUsedByte returns the last used byte on the disk based on partition info.
// This is the end of the last partition, useful for determining if data fits on smaller drive.
func (d *Disk) LastUsedByte() int64 {
	if len(d.Partitions) == 0 {
		return 0
	}

	var maxEnd int64
	sectorSize := int64(d.SectorSize)
	if sectorSize == 0 {
		sectorSize = 512 // Default sector size
	}

	for _, p := range d.Partitions {
		endByte := (p.EndLBA + 1) * sectorSize // EndLBA is inclusive, so +1
		if endByte > maxEnd {
			maxEnd = endByte
		}
	}

	// Add some buffer for GPT backup (typically 33 sectors at end)
	maxEnd += 33 * sectorSize

	return maxEnd
}

// Partition represents a partition on a disk.
type Partition struct {
	Path      string // /dev/sda1, /dev/nvme0n1p1
	Name      string // sda1, nvme0n1p1
	Number    int    // Partition number
	StartLBA  int64  // Start sector (LBA)
	EndLBA    int64  // End sector (LBA)
	SizeBytes int64  // Size in bytes
	TypeGUID  string // GPT partition type GUID
	TypeName  string // Human-readable type name (e.g., "EFI System")
	PartGUID  string // Unique partition GUID
	Label     string // GPT partition name/label
	FSType    string // Filesystem type (ntfs, fat32, ext4)
	FSLabel   string // Filesystem label
	FSUUID    string // Filesystem UUID
}

// SizeGB returns the partition size in gigabytes.
func (p *Partition) SizeGB() float64 {
	return float64(p.SizeBytes) / 1e9
}

// Well-known GPT partition type GUIDs.
const (
	GUIDEFISystem       = "C12A7328-F81F-11D2-BA4B-00A0C93EC93B"
	GUIDMSReserved      = "E3C9E316-0B5C-4DB8-817D-F92DF00215AE"
	GUIDBasicData       = "EBD0A0A2-B9E5-4433-87C0-68B6B72699C7"
	GUIDWindowsRecovery = "DE94BBA4-06D1-4D40-A16A-BFD50179D6AC"
	GUIDLinuxFilesystem = "0FC63DAF-8483-4772-8E79-3D69D8477DE4"
	GUIDLinuxSwap       = "0657FD6D-A4AB-43C4-84E5-0933C84B4F4F"
)

// TypeGUIDToName returns a human-readable name for a GPT type GUID.
func TypeGUIDToName(guid string) string {
	names := map[string]string{
		GUIDEFISystem:       "EFI System",
		GUIDMSReserved:      "Microsoft Reserved",
		GUIDBasicData:       "Basic Data",
		GUIDWindowsRecovery: "Windows Recovery",
		GUIDLinuxFilesystem: "Linux Filesystem",
		GUIDLinuxSwap:       "Linux Swap",
	}
	if name, ok := names[guid]; ok {
		return name
	}
	return "Unknown"
}
