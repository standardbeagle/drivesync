package devices

import (
	"strings"
)

// DriveSpec represents how to identify a drive.
type DriveSpec struct {
	Type  string // "internal", "boot-drive", "serial", "model", "path"
	Value string // The match value
}

// MatchDrive finds a drive matching the specification.
func MatchDrive(disks []*Disk, spec *DriveSpec, bootDevice *Disk) *Disk {
	if spec == nil {
		return nil
	}

	switch spec.Type {
	case "internal":
		return findInternalDrive(disks, bootDevice)

	case "boot-drive":
		return bootDevice

	case "serial":
		for _, disk := range disks {
			if disk.Serial == spec.Value {
				return disk
			}
		}

	case "model":
		for _, disk := range disks {
			if strings.Contains(disk.Model, spec.Value) {
				return disk
			}
		}

	case "path":
		for _, disk := range disks {
			if disk.Path == spec.Value {
				return disk
			}
		}
	}

	return nil
}

// findInternalDrive finds the first internal drive with Windows.
func findInternalDrive(disks []*Disk, bootDevice *Disk) *Disk {
	// First pass: look for Windows installation
	for _, disk := range disks {
		// Skip boot device
		if bootDevice != nil && disk.Path == bootDevice.Path {
			continue
		}
		// Skip USB/removable
		if disk.IsUSB || disk.Removable {
			continue
		}
		// Check for Windows
		if HasWindowsInstallation(disk) {
			return disk
		}
	}

	// Second pass: just find first internal drive
	for _, disk := range disks {
		if bootDevice != nil && disk.Path == bootDevice.Path {
			continue
		}
		if disk.IsUSB || disk.Removable {
			continue
		}
		return disk
	}

	return nil
}

// MatchResult contains the result of matching drives from config.
type MatchResult struct {
	Source       *Disk
	Destination  *Disk
	BootDevice   *Disk
	SourceSpec   *DriveSpec
	DestSpec     *DriveSpec
	Error        string
	AutoDetected bool   // True if source/dest were auto-detected (no config needed)
	Confidence   string // "high" = unambiguous, "medium" = best guess, "low" = needs user input
}

// AutoDetectDrives intelligently selects source and destination when the situation is unambiguous.
// Returns high confidence when there's exactly one internal drive and one external (boot) drive.
func AutoDetectDrives(disks []*Disk) *MatchResult {
	result := &MatchResult{
		AutoDetected: true,
	}

	// Detect boot device first
	ctx, _ := DetectBootContext(disks)
	if ctx != nil {
		result.BootDevice = ctx.BootDevice
	}

	// Categorize drives
	var internalDrives []*Disk
	var externalDrives []*Disk

	for _, disk := range disks {
		// Skip the boot device from categorization (it's always destination)
		if result.BootDevice != nil && disk.Path == result.BootDevice.Path {
			continue
		}

		if disk.IsUSB || disk.Removable {
			externalDrives = append(externalDrives, disk)
		} else {
			internalDrives = append(internalDrives, disk)
		}
	}

	// Case 1: Exactly one internal drive + boot device is external
	// This is the ideal case - completely unambiguous
	if len(internalDrives) == 1 && result.BootDevice != nil {
		result.Source = internalDrives[0]
		result.Destination = result.BootDevice
		result.Confidence = "high"
		return result
	}

	// Case 2: Multiple internal drives - pick the one with Windows
	if len(internalDrives) > 1 && result.BootDevice != nil {
		for _, disk := range internalDrives {
			if HasWindowsInstallation(disk) {
				result.Source = disk
				result.Destination = result.BootDevice
				result.Confidence = "medium" // User should confirm which internal drive
				return result
			}
		}
		// No Windows found, pick largest internal drive
		var largest *Disk
		for _, disk := range internalDrives {
			if largest == nil || disk.SizeBytes > largest.SizeBytes {
				largest = disk
			}
		}
		result.Source = largest
		result.Destination = result.BootDevice
		result.Confidence = "low" // Multiple internals, no clear winner
		return result
	}

	// Case 3: No internal drives
	if len(internalDrives) == 0 {
		result.Error = "no internal drive found to clone from"
		result.Confidence = "low"
		return result
	}

	// Case 4: No boot device detected
	if result.BootDevice == nil {
		result.Source = internalDrives[0]
		if len(externalDrives) == 1 {
			result.Destination = externalDrives[0]
			result.Confidence = "medium"
		} else if len(externalDrives) > 1 {
			result.Destination = externalDrives[0] // Pick first
			result.Confidence = "low"
		} else {
			result.Error = "no destination drive found"
			result.Confidence = "low"
		}
		return result
	}

	return result
}

// MatchDrivesFromConfig finds source and destination drives based on specs.
// If both specs are nil, uses intelligent auto-detection.
func MatchDrivesFromConfig(disks []*Disk, sourceSpec, destSpec *DriveSpec) (*MatchResult, error) {
	// If no specs provided, use auto-detection
	if sourceSpec == nil && destSpec == nil {
		return AutoDetectDrives(disks), nil
	}

	result := &MatchResult{
		SourceSpec: sourceSpec,
		DestSpec:   destSpec,
	}

	// Detect boot device first
	ctx, _ := DetectBootContext(disks)
	if ctx != nil {
		result.BootDevice = ctx.BootDevice
	}

	// Match source - use auto-detect if not specified
	if sourceSpec != nil {
		result.Source = MatchDrive(disks, sourceSpec, result.BootDevice)
		if result.Source == nil {
			result.Error = "source drive not found"
		}
	} else {
		// Auto-detect source (first internal drive)
		result.Source = findInternalDrive(disks, result.BootDevice)
		if result.Source == nil {
			result.Error = "no internal drive found"
		}
		result.AutoDetected = true
	}

	// Match destination - use boot-drive if not specified
	if destSpec != nil {
		result.Destination = MatchDrive(disks, destSpec, result.BootDevice)
		if result.Destination == nil {
			if result.Error == "" {
				result.Error = "destination drive not found"
			} else {
				result.Error += "; destination drive not found"
			}
		}
	} else {
		// Auto-detect destination (boot drive)
		result.Destination = result.BootDevice
		if result.Destination == nil {
			if result.Error == "" {
				result.Error = "boot drive not detected"
			} else {
				result.Error += "; boot drive not detected"
			}
		}
		result.AutoDetected = true
	}

	// Validate source != destination
	if result.Source != nil && result.Destination != nil {
		if result.Source.Path == result.Destination.Path {
			result.Error = "source and destination cannot be the same drive"
		}
	}

	// Set confidence based on what was auto-detected
	if result.AutoDetected && result.Error == "" {
		result.Confidence = "medium" // Partial auto-detect
	}

	return result, nil
}
