package devices

import (
	"os"
	"path/filepath"
	"strings"
)

// BootContext describes the boot environment for self-overwrite detection.
type BootContext struct {
	BootDevice       *Disk // Drive we booted from
	InternalDrive    *Disk // Detected Windows installation (if any)
	CanSelfOverwrite bool  // Self-overwrite mode is available
}

// DetectBootContext analyzes the boot environment.
func DetectBootContext(disks []*Disk) (*BootContext, error) {
	ctx := &BootContext{}

	// Find our boot device
	bootDevPath, err := findBootDevice()
	if err != nil {
		return ctx, nil // Not an error, just can't determine
	}

	// Find the disk containing the boot device
	for _, disk := range disks {
		if strings.HasPrefix(bootDevPath, disk.Path) || disk.Path == bootDevPath {
			ctx.BootDevice = disk
			break
		}
	}

	if ctx.BootDevice == nil {
		return ctx, nil
	}

	// Is boot device USB/removable?
	if !ctx.BootDevice.IsUSB && !ctx.BootDevice.Removable {
		return ctx, nil // Boot from internal drive, no self-overwrite
	}

	// Find internal Windows installation
	for _, disk := range disks {
		if disk.Path == ctx.BootDevice.Path {
			continue
		}
		if disk.IsUSB || disk.Removable {
			continue
		}

		if hasWindowsInstallation(disk) {
			ctx.InternalDrive = disk
			ctx.CanSelfOverwrite = true
			break
		}
	}

	return ctx, nil
}

// findBootDevice determines which device we booted from.
func findBootDevice() (string, error) {
	// Method 1: Check EFI system partition mount
	if efiDev := findEFIMountDevice(); efiDev != "" {
		return getParentDevice(efiDev), nil
	}

	// Method 2: Parse /proc/cmdline for root device
	if rootDev := parseRootFromCmdline(); rootDev != "" {
		return getParentDevice(rootDev), nil
	}

	// Method 3: Check /boot mount point
	if bootDev := findMountDevice("/boot"); bootDev != "" {
		return getParentDevice(bootDev), nil
	}

	return "", nil
}

// findEFIMountDevice finds the device mounted at /boot/efi or /efi.
func findEFIMountDevice() string {
	for _, mountPoint := range []string{"/boot/efi", "/efi", "/sys/firmware/efi"} {
		if dev := findMountDevice(mountPoint); dev != "" {
			return dev
		}
	}
	return ""
}

// findMountDevice finds the device mounted at a given path.
func findMountDevice(mountPath string) string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPath {
			return fields[0]
		}
	}
	return ""
}

// parseRootFromCmdline extracts the root device from kernel command line.
func parseRootFromCmdline() string {
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return ""
	}

	cmdline := string(data)
	for _, part := range strings.Fields(cmdline) {
		if strings.HasPrefix(part, "root=") {
			root := strings.TrimPrefix(part, "root=")
			// Handle UUID= and PARTUUID= formats
			if strings.HasPrefix(root, "UUID=") || strings.HasPrefix(root, "PARTUUID=") {
				return resolveDeviceByUUID(root)
			}
			return root
		}
	}
	return ""
}

// resolveDeviceByUUID resolves a UUID= or PARTUUID= to a device path.
func resolveDeviceByUUID(spec string) string {
	// Check /dev/disk/by-uuid or /dev/disk/by-partuuid
	var dir string
	var id string

	if strings.HasPrefix(spec, "UUID=") {
		dir = "/dev/disk/by-uuid"
		id = strings.TrimPrefix(spec, "UUID=")
	} else if strings.HasPrefix(spec, "PARTUUID=") {
		dir = "/dev/disk/by-partuuid"
		id = strings.TrimPrefix(spec, "PARTUUID=")
	} else {
		return ""
	}

	linkPath := filepath.Join(dir, id)
	target, err := os.Readlink(linkPath)
	if err != nil {
		return ""
	}

	// Resolve relative symlink
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return target
	}
	return resolved
}

// getParentDevice returns the parent device for a partition.
// e.g., /dev/sda1 -> /dev/sda, /dev/nvme0n1p1 -> /dev/nvme0n1
func getParentDevice(partPath string) string {
	// Handle NVMe style: nvme0n1p1 -> nvme0n1
	base := filepath.Base(partPath)
	if strings.Contains(base, "nvme") && strings.Contains(base, "p") {
		idx := strings.LastIndex(base, "p")
		if idx > 0 {
			return filepath.Dir(partPath) + "/" + base[:idx]
		}
	}

	// Handle SD/MMC style: mmcblk0p1 -> mmcblk0
	if strings.Contains(base, "mmcblk") && strings.Contains(base, "p") {
		idx := strings.LastIndex(base, "p")
		if idx > 0 {
			return filepath.Dir(partPath) + "/" + base[:idx]
		}
	}

	// Handle standard style: sda1 -> sda
	// Remove trailing digits
	i := len(base) - 1
	for i >= 0 && base[i] >= '0' && base[i] <= '9' {
		i--
	}
	if i < len(base)-1 {
		return filepath.Dir(partPath) + "/" + base[:i+1]
	}

	return partPath
}

// hasWindowsInstallation checks if a disk has a Windows installation.
func hasWindowsInstallation(disk *Disk) bool {
	// Check partitions for Windows indicators
	hasEFI := false
	hasNTFS := false

	for _, part := range disk.Partitions {
		// Check for EFI System Partition
		if part.TypeGUID == GUIDEFISystem {
			hasEFI = true
			// Try to mount and check for Windows boot files
			if hasWindowsBootFiles(part.Path) {
				return true
			}
		}

		// Check for NTFS partition (likely Windows C:)
		if part.FSType == "ntfs" {
			hasNTFS = true
		}

		// Check for Windows Recovery partition
		if part.TypeGUID == GUIDWindowsRecovery {
			return true
		}
	}

	// EFI + NTFS is a strong indicator
	return hasEFI && hasNTFS
}

// hasWindowsBootFiles checks if an EFI partition has Windows boot files.
func hasWindowsBootFiles(partPath string) bool {
	// This is a heuristic - we check if we can read Windows boot manager
	// In a real implementation, we'd mount the partition temporarily

	// For now, just check if the partition exists and is FAT32
	info := GetBlkidInfo(partPath)
	return info.FSType == "vfat" || info.FSType == "fat32"
}
