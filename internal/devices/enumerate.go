//go:build !windows

package devices

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Enumerate discovers all block devices and returns a list of disks.
// It reads from /sys/block to find devices and their properties.
func Enumerate() ([]*Disk, error) {
	return EnumerateFromPath("/sys/block")
}

// EnumerateFromPath discovers block devices from a custom sysfs path.
// This is useful for testing with mock sysfs data.
func EnumerateFromPath(sysBlockPath string) ([]*Disk, error) {
	entries, err := os.ReadDir(sysBlockPath)
	if err != nil {
		return nil, err
	}

	var disks []*Disk
	for _, entry := range entries {
		name := entry.Name()

		// Skip virtual devices (loop, ram, etc.)
		if isVirtualDevice(name) {
			continue
		}

		diskPath := filepath.Join(sysBlockPath, name)
		disk, err := parseDisk(diskPath, name)
		if err != nil {
			// Skip devices we can't read
			continue
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

// isVirtualDevice returns true for devices we should skip.
func isVirtualDevice(name string) bool {
	// Skip loop devices, ram disks, device-mapper, etc.
	prefixes := []string{"loop", "ram", "dm-", "sr", "fd", "md", "zram"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// parseDisk reads sysfs to populate disk information.
func parseDisk(sysfsPath, name string) (*Disk, error) {
	disk := &Disk{
		Name:      name,
		Path:      "/dev/" + name,
		SysfsPath: sysfsPath,
	}

	// Read size in 512-byte sectors
	if sizeStr, err := readSysfsFile(sysfsPath, "size"); err == nil {
		if sectors, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			disk.SizeSectors = sectors
			disk.SizeBytes = sectors * 512 // sysfs reports in 512-byte sectors
		}
	}

	// Read logical sector size
	disk.SectorSize = 512 // default
	if sectorSizeStr, err := readSysfsFile(sysfsPath, "queue/logical_block_size"); err == nil {
		if size, err := strconv.Atoi(sectorSizeStr); err == nil {
			disk.SectorSize = size
		}
	}

	// Read removable flag
	if removableStr, err := readSysfsFile(sysfsPath, "removable"); err == nil {
		disk.Removable = removableStr == "1"
	}

	// Determine drive type (SSD vs HDD)
	if rotationalStr, err := readSysfsFile(sysfsPath, "queue/rotational"); err == nil {
		if rotationalStr == "0" {
			disk.DriveType = DriveTypeSSD
		} else {
			disk.DriveType = DriveTypeHDD
		}
	} else {
		disk.DriveType = DriveTypeUnknown
	}

	// Read device model and vendor
	disk.Model = readSysfsFileOrEmpty(sysfsPath, "device/model")
	disk.Vendor = readSysfsFileOrEmpty(sysfsPath, "device/vendor")
	disk.Serial = readSysfsFileOrEmpty(sysfsPath, "device/serial")

	// Determine transport type
	disk.Transport = detectTransport(sysfsPath, name)
	disk.IsUSB = disk.Transport == TransportUSB

	// If USB, try to get better names from USB descriptors
	if disk.IsUSB {
		if usbManufacturer := readSysfsFileOrEmpty(sysfsPath, "device/../../manufacturer"); usbManufacturer != "" {
			disk.Vendor = usbManufacturer
		}
		if usbProduct := readSysfsFileOrEmpty(sysfsPath, "device/../../product"); usbProduct != "" {
			disk.Model = usbProduct
		}
	}

	// Clean up model/vendor strings
	disk.Model = strings.TrimSpace(disk.Model)
	disk.Vendor = strings.TrimSpace(disk.Vendor)

	// Enumerate partitions
	disk.Partitions = enumeratePartitions(disk.Path, sysfsPath, name, disk.SectorSize)

	return disk, nil
}

// detectTransport determines how the drive is connected.
func detectTransport(sysfsPath, name string) Transport {
	// NVMe devices have predictable names
	if strings.HasPrefix(name, "nvme") {
		return TransportNVMe
	}

	// MMC/SD cards
	if strings.HasPrefix(name, "mmcblk") {
		return TransportMMC
	}

	// Check the device path for USB or ATA indicators
	deviceLink, err := os.Readlink(filepath.Join(sysfsPath, "device"))
	if err == nil {
		fullPath := filepath.Join(sysfsPath, "device", deviceLink)
		if strings.Contains(fullPath, "/usb") {
			return TransportUSB
		}
		if strings.Contains(fullPath, "/ata") {
			return TransportSATA
		}
	}

	// Try reading the symlink of the device itself
	blockLink, err := filepath.EvalSymlinks(sysfsPath)
	if err == nil {
		if strings.Contains(blockLink, "/usb") {
			return TransportUSB
		}
		if strings.Contains(blockLink, "/ata") {
			return TransportSATA
		}
	}

	return TransportUnknown
}

// readSysfsFile reads a sysfs file and returns its trimmed contents.
func readSysfsFile(basePath, relPath string) (string, error) {
	fullPath := filepath.Join(basePath, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readSysfsFileOrEmpty reads a sysfs file, returning empty string on error.
func readSysfsFileOrEmpty(basePath, relPath string) string {
	content, _ := readSysfsFile(basePath, relPath)
	return content
}

// enumeratePartitions discovers partitions for a disk using sgdisk.
func enumeratePartitions(devPath, sysfsPath, diskName string, sectorSize int) []*Partition {
	// Try sgdisk first for GPT partition info
	partitions := parseGPTPartitions(devPath, diskName, sectorSize)
	if len(partitions) > 0 {
		return partitions
	}

	// Fallback: enumerate from sysfs (won't have LBA info, but will find partitions)
	return enumeratePartitionsFromSysfs(sysfsPath, diskName, sectorSize)
}

// parseGPTPartitions uses sgdisk to get GPT partition info.
func parseGPTPartitions(devPath, diskName string, sectorSize int) []*Partition {
	// Run sgdisk -p to get partition info
	cmd := exec.Command("sgdisk", "-p", devPath)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var partitions []*Partition
	lines := strings.Split(string(output), "\n")

	// Parse partition lines - format:
	// Number  Start (sector)    End (sector)  Size       Code  Name
	//    1            2048          206847   100.0 MiB   EF00  EFI system partition
	partRegex := regexp.MustCompile(`^\s*(\d+)\s+(\d+)\s+(\d+)\s+[\d.]+\s+\S+\s+\S+\s+(.*)$`)

	for _, line := range lines {
		matches := partRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		partNum, _ := strconv.Atoi(matches[1])
		startLBA, _ := strconv.ParseInt(matches[2], 10, 64)
		endLBA, _ := strconv.ParseInt(matches[3], 10, 64)
		label := strings.TrimSpace(matches[4])

		// Construct partition device path
		partPath := partitionDevicePath(diskName, partNum)

		// Calculate size
		sectorSz := int64(sectorSize)
		if sectorSz == 0 {
			sectorSz = 512
		}
		sizeBytes := (endLBA - startLBA + 1) * sectorSz

		part := &Partition{
			Path:      partPath,
			Name:      strings.TrimPrefix(partPath, "/dev/"),
			Number:    partNum,
			StartLBA:  startLBA,
			EndLBA:    endLBA,
			SizeBytes: sizeBytes,
			Label:     label,
		}

		// Get filesystem info via blkid
		blkidInfo := GetBlkidInfo(partPath)
		part.FSType = blkidInfo.FSType
		part.FSLabel = blkidInfo.Label
		part.FSUUID = blkidInfo.UUID
		part.TypeGUID = blkidInfo.PartTypeGUID
		if part.TypeGUID != "" {
			part.TypeName = TypeGUIDToName(part.TypeGUID)
		}

		partitions = append(partitions, part)
	}

	return partitions
}

// enumeratePartitionsFromSysfs finds partitions from sysfs without LBA info.
func enumeratePartitionsFromSysfs(sysfsPath, diskName string, sectorSize int) []*Partition {
	entries, err := os.ReadDir(sysfsPath)
	if err != nil {
		return nil
	}

	var partitions []*Partition
	for _, entry := range entries {
		name := entry.Name()
		// Partition directories start with the disk name
		if !strings.HasPrefix(name, diskName) {
			continue
		}
		// Must have a partition number suffix
		suffix := strings.TrimPrefix(name, diskName)
		// Handle nvme (nvme0n1p1) vs sda (sda1)
		suffix = strings.TrimPrefix(suffix, "p")
		if suffix == "" {
			continue
		}
		partNum, err := strconv.Atoi(suffix)
		if err != nil {
			continue
		}

		partPath := filepath.Join(sysfsPath, name)
		devPath := "/dev/" + name

		// Read partition size
		var sizeBytes int64
		if sizeStr, err := readSysfsFile(partPath, "size"); err == nil {
			if sectors, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				sizeBytes = sectors * 512
			}
		}

		// Read start sector
		var startLBA int64
		if startStr, err := readSysfsFile(partPath, "start"); err == nil {
			startLBA, _ = strconv.ParseInt(startStr, 10, 64)
		}

		// Calculate end LBA from start and size
		sectorSz := int64(sectorSize)
		if sectorSz == 0 {
			sectorSz = 512
		}
		endLBA := startLBA + (sizeBytes / sectorSz) - 1

		part := &Partition{
			Path:      devPath,
			Name:      name,
			Number:    partNum,
			StartLBA:  startLBA,
			EndLBA:    endLBA,
			SizeBytes: sizeBytes,
		}

		// Get filesystem info via blkid
		blkidInfo := GetBlkidInfo(devPath)
		part.FSType = blkidInfo.FSType
		part.FSLabel = blkidInfo.Label
		part.FSUUID = blkidInfo.UUID
		part.TypeGUID = blkidInfo.PartTypeGUID
		if part.TypeGUID != "" {
			part.TypeName = TypeGUIDToName(part.TypeGUID)
		}

		partitions = append(partitions, part)
	}

	return partitions
}

// partitionDevicePath returns the device path for a partition.
func partitionDevicePath(diskName string, partNum int) string {
	// NVMe and MMC use "p" separator: nvme0n1p1, mmcblk0p1
	if strings.HasPrefix(diskName, "nvme") || strings.HasPrefix(diskName, "mmcblk") {
		return "/dev/" + diskName + "p" + strconv.Itoa(partNum)
	}
	// SATA/USB use no separator: sda1, sdb1
	return "/dev/" + diskName + strconv.Itoa(partNum)
}
