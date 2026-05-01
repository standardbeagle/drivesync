//go:build windows

package vss

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Snapshot represents a VSS shadow copy snapshot.
type Snapshot struct {
	SnapshotID  string
	DevicePath  string
	VolumeName  string
	backupComp  *ole.IDispatch
	snapshotSet *ole.GUID
	initialized bool
}

// CreateSnapshot creates a VSS shadow copy of the specified volume.
// volumePath should be like "C:\" or "\\?\Volume{GUID}\"
func CreateSnapshot(volumePath string) (*Snapshot, error) {
	// Initialize COM
	err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	if err != nil {
		return nil, fmt.Errorf("COM initialization failed: %w", err)
	}

	// Create VSS backup components
	unknown, err := oleutil.CreateObject("VssBackupComponents")
	if err != nil {
		ole.CoUninitialize()
		return nil, fmt.Errorf("failed to create VssBackupComponents: %w", err)
	}

	backupComp, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		ole.CoUninitialize()
		return nil, fmt.Errorf("QueryInterface failed: %w", err)
	}

	// Initialize for backup
	_, err = oleutil.CallMethod(backupComp, "InitializeForBackup")
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("InitializeForBackup failed: %w", err)
	}

	// Set backup state
	_, err = oleutil.CallMethod(backupComp, "SetBackupState",
		false, // not full backup
		true,  // bootable system state
		5,     // VSS_BT_COPY
		false) // not partial file support
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("SetBackupState failed: %w", err)
	}

	// Start snapshot set
	result, err := oleutil.CallMethod(backupComp, "StartSnapshotSet")
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("StartSnapshotSet failed: %w", err)
	}

	snapshotSetID := result.Value().(string)
	snapshotSetGUID, err := ole.CLSIDFromString(snapshotSetID)
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("invalid snapshot set GUID: %w", err)
	}

	// Normalize volume path (ensure trailing backslash)
	if len(volumePath) > 0 && volumePath[len(volumePath)-1] != '\\' {
		volumePath += "\\"
	}

	// Add volume to snapshot set
	result, err = oleutil.CallMethod(backupComp, "AddToSnapshotSet",
		volumePath,
		ole.GUID{}) // null GUID for provider
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("AddToSnapshotSet failed: %w", err)
	}

	snapshotID := result.Value().(string)

	// Prepare for backup (gathers writer metadata)
	_, err = oleutil.CallMethod(backupComp, "PrepareForBackup")
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("PrepareForBackup failed: %w", err)
	}

	// Create the snapshot (this is the actual VSS operation)
	_, err = oleutil.CallMethod(backupComp, "DoSnapshotSet")
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("DoSnapshotSet failed: %w", err)
	}

	// Get snapshot properties to find device path
	result, err = oleutil.CallMethod(backupComp, "GetSnapshotProperties", snapshotID)
	if err != nil {
		backupComp.Release()
		ole.CoUninitialize()
		return nil, fmt.Errorf("GetSnapshotProperties failed: %w", err)
	}

	props := result.ToIDispatch()
	devicePathVar := oleutil.MustGetProperty(props, "SnapshotDeviceObject")
	devicePath := devicePathVar.ToString()
	props.Release()

	snapshot := &Snapshot{
		SnapshotID:  snapshotID,
		DevicePath:  devicePath,
		VolumeName:  volumePath,
		backupComp:  backupComp,
		snapshotSet: snapshotSetGUID,
		initialized: true,
	}

	return snapshot, nil
}

// Delete removes the VSS snapshot and cleans up resources.
func (s *Snapshot) Delete() error {
	if !s.initialized {
		return nil
	}

	var lastErr error

	// Delete the snapshot set
	if s.snapshotSet != nil {
		_, err := oleutil.CallMethod(s.backupComp, "DeleteSnapshots",
			s.SnapshotID,
			2,    // VSS_OBJECT_SNAPSHOT
			true, // force delete
			nil,  // deleted snapshots count
			nil)  // non-deleted snapshot ID
		if err != nil {
			lastErr = fmt.Errorf("DeleteSnapshots failed: %w", err)
		}
	}

	// Release backup components
	if s.backupComp != nil {
		s.backupComp.Release()
		s.backupComp = nil
	}

	// Uninitialize COM
	ole.CoUninitialize()
	s.initialized = false

	return lastErr
}

// GetPhysicalDrivePath converts a volume path to physical drive path.
// E.g., "C:\" -> "\\.\PhysicalDrive0"
func GetPhysicalDriveForVolume(volumePath string) (string, error) {
	// Normalize volume path
	if len(volumePath) == 2 && volumePath[1] == ':' {
		volumePath += "\\"
	}

	// Convert to volume name format
	volNameBuf := make([]uint16, syscall.MAX_PATH)
	volNamePtr := &volNameBuf[0]

	volPathPtr, err := syscall.UTF16PtrFromString(volumePath)
	if err != nil {
		return "", err
	}

	// Get volume name (\\?\Volume{GUID}\)
	ret, _, _ := getVolumeNameForVolumeMountPoint.Call(
		uintptr(unsafe.Pointer(volPathPtr)),
		uintptr(unsafe.Pointer(volNamePtr)),
		uintptr(len(volNameBuf)),
	)

	if ret == 0 {
		return "", fmt.Errorf("GetVolumeNameForVolumeMountPoint failed")
	}

	volumeName := syscall.UTF16ToString(volNameBuf)

	// Remove trailing backslash for CreateFile
	if len(volumeName) > 0 && volumeName[len(volumeName)-1] == '\\' {
		volumeName = volumeName[:len(volumeName)-1]
	}

	// Open the volume
	volHandle, err := syscall.CreateFile(
		volPathPtr,
		0, // No access needed
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		0,
		0,
	)

	if err != nil {
		return "", fmt.Errorf("CreateFile on volume failed: %w", err)
	}
	defer syscall.CloseHandle(volHandle)

	// Get volume disk extents
	var extents volumeDiskExtents
	var bytesReturned uint32

	err = syscall.DeviceIoControl(
		volHandle,
		IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS,
		nil,
		0,
		(*byte)(unsafe.Pointer(&extents)),
		uint32(unsafe.Sizeof(extents)),
		&bytesReturned,
		nil,
	)

	if err != nil {
		return "", fmt.Errorf("IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS failed: %w", err)
	}

	if extents.NumberOfDiskExtents == 0 {
		return "", fmt.Errorf("no disk extents found")
	}

	// Return first physical drive (most common case)
	diskNumber := extents.Extents[0].DiskNumber
	return fmt.Sprintf("\\\\.\\PhysicalDrive%d", diskNumber), nil
}

// Windows API declarations
var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	getVolumeNameForVolumeMountPoint = kernel32.NewProc("GetVolumeNameForVolumeMountPointW")
)

const (
	IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS = 0x00560000
)

type volumeDiskExtents struct {
	NumberOfDiskExtents uint32
	Extents             [1]diskExtent
}

type diskExtent struct {
	DiskNumber     uint32
	StartingOffset int64
	ExtentLength   int64
}

// WaitForSnapshot waits for the snapshot to be fully created and accessible.
func (s *Snapshot) WaitForSnapshot(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to open the snapshot device
		pathPtr, err := syscall.UTF16PtrFromString(s.DevicePath)
		if err != nil {
			return err
		}

		handle, err := syscall.CreateFile(
			pathPtr,
			syscall.GENERIC_READ,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
			nil,
			syscall.OPEN_EXISTING,
			0,
			0,
		)

		if err == nil {
			syscall.CloseHandle(handle)
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("snapshot device not ready after %v", timeout)
}
