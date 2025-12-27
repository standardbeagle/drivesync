//go:build !windows

package devices

import (
	"os"
	"path/filepath"
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
