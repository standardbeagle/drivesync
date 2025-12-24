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
		if hasWindowsInstallation(disk) {
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
	Source      *Disk
	Destination *Disk
	BootDevice  *Disk
	SourceSpec  *DriveSpec
	DestSpec    *DriveSpec
	Error       string
}

// MatchDrivesFromConfig finds source and destination drives based on specs.
func MatchDrivesFromConfig(disks []*Disk, sourceSpec, destSpec *DriveSpec) (*MatchResult, error) {
	result := &MatchResult{
		SourceSpec: sourceSpec,
		DestSpec:   destSpec,
	}

	// Detect boot device first
	ctx, _ := DetectBootContext(disks)
	if ctx != nil {
		result.BootDevice = ctx.BootDevice
	}

	// Match source
	if sourceSpec != nil {
		result.Source = MatchDrive(disks, sourceSpec, result.BootDevice)
		if result.Source == nil {
			result.Error = "source drive not found"
		}
	}

	// Match destination
	if destSpec != nil {
		result.Destination = MatchDrive(disks, destSpec, result.BootDevice)
		if result.Destination == nil {
			if result.Error == "" {
				result.Error = "destination drive not found"
			} else {
				result.Error += "; destination drive not found"
			}
		}
	}

	// Validate source != destination
	if result.Source != nil && result.Destination != nil {
		if result.Source.Path == result.Destination.Path {
			result.Error = "source and destination cannot be the same drive"
		}
	}

	return result, nil
}
