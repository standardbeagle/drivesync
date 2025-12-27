//go:build !windows

package vss

import (
	"errors"
	"time"
)

// Snapshot represents a VSS shadow copy snapshot (stub for non-Windows).
type Snapshot struct {
	SnapshotID string
	DevicePath string
	VolumeName string
}

// CreateSnapshot is not supported on non-Windows platforms.
func CreateSnapshot(volumePath string) (*Snapshot, error) {
	return nil, errors.New("VSS snapshots are only supported on Windows")
}

// Delete is a no-op on non-Windows platforms.
func (s *Snapshot) Delete() error {
	return nil
}

// GetPhysicalDriveForVolume is not supported on non-Windows platforms.
func GetPhysicalDriveForVolume(volumePath string) (string, error) {
	return "", errors.New("volume to physical drive mapping is only supported on Windows")
}

// WaitForSnapshot is a no-op on non-Windows platforms.
func (s *Snapshot) WaitForSnapshot(timeout time.Duration) error {
	return errors.New("VSS snapshots are only supported on Windows")
}
