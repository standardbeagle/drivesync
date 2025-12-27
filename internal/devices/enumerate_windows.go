//go:build windows

package devices

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	setupapi = syscall.NewLazyDLL("setupapi.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procSetupDiGetClassDevsW          = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces   = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList  = setupapi.NewProc("SetupDiDestroyDeviceInfoList")

	procCreateFileW    = kernel32.NewProc("CreateFileW")
	procDeviceIoControl = kernel32.NewProc("DeviceIoControl")
	procCloseHandle    = kernel32.NewProc("CloseHandle")
)

// GUIDs
var (
	GUID_DEVINTERFACE_DISK = syscall.GUID{
		Data1: 0x53f56307,
		Data2: 0xb6bf,
		Data3: 0x11d0,
		Data4: [8]byte{0x94, 0xf2, 0x00, 0xa0, 0xc9, 0x1e, 0xfb, 0x8b},
	}
)

// Constants
const (
	DIGCF_PRESENT         = 0x00000002
	DIGCF_DEVICEINTERFACE = 0x00000010

	GENERIC_READ  = 0x80000000
	GENERIC_WRITE = 0x40000000
	FILE_SHARE_READ  = 0x00000001
	FILE_SHARE_WRITE = 0x00000002
	OPEN_EXISTING = 3

	IOCTL_DISK_GET_DRIVE_GEOMETRY_EX = 0x000700A0
	IOCTL_STORAGE_GET_DEVICE_NUMBER  = 0x002D1080
	IOCTL_STORAGE_QUERY_PROPERTY     = 0x002D1400

	PropertyStandardQuery = 0
	StorageDeviceProperty = 0

	INVALID_HANDLE_VALUE = ^syscall.Handle(0)
)

// Structures
type SP_DEVICE_INTERFACE_DATA struct {
	CbSize             uint32
	InterfaceClassGuid syscall.GUID
	Flags              uint32
	Reserved           uintptr
}

type SP_DEVICE_INTERFACE_DETAIL_DATA struct {
	CbSize     uint32
	DevicePath [1]uint16
}

type DISK_GEOMETRY_EX struct {
	Geometry DISK_GEOMETRY
	DiskSize int64
	Data     [1]byte
}

type DISK_GEOMETRY struct {
	Cylinders         int64
	MediaType         uint32
	TracksPerCylinder uint32
	SectorsPerTrack   uint32
	BytesPerSector    uint32
}

type STORAGE_DEVICE_NUMBER struct {
	DeviceType      uint32
	DeviceNumber    uint32
	PartitionNumber uint32
}

type STORAGE_PROPERTY_QUERY struct {
	PropertyId           uint32
	QueryType            uint32
	AdditionalParameters [1]byte
}

type STORAGE_DEVICE_DESCRIPTOR struct {
	Version               uint32
	Size                  uint32
	DeviceType            byte
	DeviceTypeModifier    byte
	RemovableMedia        byte
	CommandQueueing       byte
	VendorIdOffset        uint32
	ProductIdOffset       uint32
	ProductRevisionOffset uint32
	SerialNumberOffset    uint32
	BusType               uint32
	RawPropertiesLength   uint32
	RawDeviceProperties   [1]byte
}

// Bus types
const (
	BusTypeUnknown = 0
	BusTypeScsi    = 1
	BusTypeAtapi   = 2
	BusTypeAta     = 3
	BusType1394    = 4
	BusTypeSsa     = 5
	BusTypeFibre   = 6
	BusTypeUsb     = 7
	BusTypeRAID    = 8
	BusTypeiScsi   = 9
	BusTypeSas     = 10
	BusTypeSata    = 11
	BusTypeSd      = 12
	BusTypeMmc     = 13
	BusTypeNvme    = 17
)

// Enumerate discovers all disk devices on Windows using Win32 APIs.
func Enumerate() ([]*Disk, error) {
	return EnumerateWin32()
}

// EnumerateWin32 discovers disks using SetupAPI and DeviceIoControl.
func EnumerateWin32() ([]*Disk, error) {
	// Get device info set for disk interfaces
	hDevInfo, _, err := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_DISK)),
		0,
		0,
		DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
	)

	if syscall.Handle(hDevInfo) == INVALID_HANDLE_VALUE {
		return nil, fmt.Errorf("SetupDiGetClassDevsW failed: %w", err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(hDevInfo)

	// First pass: collect all device paths (fast)
	var devicePaths []string
	var index uint32 = 0

	for {
		var interfaceData SP_DEVICE_INTERFACE_DATA
		interfaceData.CbSize = uint32(unsafe.Sizeof(interfaceData))

		ret, _, _ := procSetupDiEnumDeviceInterfaces.Call(
			hDevInfo,
			0,
			uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_DISK)),
			uintptr(index),
			uintptr(unsafe.Pointer(&interfaceData)),
		)

		if ret == 0 {
			break
		}

		var requiredSize uint32
		procSetupDiGetDeviceInterfaceDetailW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&interfaceData)),
			0,
			0,
			uintptr(unsafe.Pointer(&requiredSize)),
			0,
		)

		if requiredSize == 0 {
			index++
			continue
		}

		detailBuf := make([]byte, requiredSize)
		detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&detailBuf[0]))
		if unsafe.Sizeof(uintptr(0)) == 8 {
			detail.CbSize = 8
		} else {
			detail.CbSize = 6
		}

		ret, _, _ = procSetupDiGetDeviceInterfaceDetailW.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&interfaceData)),
			uintptr(unsafe.Pointer(detail)),
			uintptr(requiredSize),
			0,
			0,
		)

		if ret != 0 {
			devicePath := syscall.UTF16ToString((*[1024]uint16)(unsafe.Pointer(&detail.DevicePath))[:])
			devicePaths = append(devicePaths, devicePath)
		}

		index++
	}

	// Second pass: query disk info in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	var disks []*Disk

	for _, path := range devicePaths {
		wg.Add(1)
		go func(devicePath string) {
			defer wg.Done()
			disk, err := getDiskInfo(devicePath)
			if err == nil && disk != nil {
				mu.Lock()
				disks = append(disks, disk)
				mu.Unlock()
			}
		}(path)
	}

	wg.Wait()
	return disks, nil
}

// getDiskInfo opens a disk and retrieves its properties.
func getDiskInfo(devicePath string) (*Disk, error) {
	pathPtr, _ := syscall.UTF16PtrFromString(devicePath)

	// Use 0 access rights - just need to send IOCTLs, not read data
	// This is much faster and doesn't block on problematic disks
	handle, _, err := procCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0, // No access rights needed for IOCTL queries
		FILE_SHARE_READ|FILE_SHARE_WRITE,
		0,
		OPEN_EXISTING,
		0,
		0,
	)

	if syscall.Handle(handle) == INVALID_HANDLE_VALUE {
		return nil, fmt.Errorf("CreateFile failed: %w", err)
	}
	defer procCloseHandle.Call(handle)

	disk := &Disk{
		Path: devicePath,
	}

	// Get device number
	var deviceNumber STORAGE_DEVICE_NUMBER
	var bytesReturned uint32

	ret, _, _ := procDeviceIoControl.Call(
		handle,
		IOCTL_STORAGE_GET_DEVICE_NUMBER,
		0, 0,
		uintptr(unsafe.Pointer(&deviceNumber)),
		uintptr(unsafe.Sizeof(deviceNumber)),
		uintptr(unsafe.Pointer(&bytesReturned)),
		0,
	)

	if ret != 0 {
		disk.Name = fmt.Sprintf("disk%d", deviceNumber.DeviceNumber)
		// Also set the PhysicalDrive path for actual disk operations
		disk.Path = fmt.Sprintf("\\\\.\\PhysicalDrive%d", deviceNumber.DeviceNumber)
	}

	// Get disk geometry (size)
	var geometry DISK_GEOMETRY_EX
	ret, _, _ = procDeviceIoControl.Call(
		handle,
		IOCTL_DISK_GET_DRIVE_GEOMETRY_EX,
		0, 0,
		uintptr(unsafe.Pointer(&geometry)),
		uintptr(unsafe.Sizeof(geometry)),
		uintptr(unsafe.Pointer(&bytesReturned)),
		0,
	)

	if ret != 0 {
		disk.SizeBytes = geometry.DiskSize
		disk.SizeSectors = geometry.DiskSize / 512
		disk.SectorSize = int(geometry.Geometry.BytesPerSector)
	}

	// Get device descriptor (vendor, model, serial, bus type)
	query := STORAGE_PROPERTY_QUERY{
		PropertyId: StorageDeviceProperty,
		QueryType:  PropertyStandardQuery,
	}

	descBuf := make([]byte, 1024)
	ret, _, _ = procDeviceIoControl.Call(
		handle,
		IOCTL_STORAGE_QUERY_PROPERTY,
		uintptr(unsafe.Pointer(&query)),
		uintptr(unsafe.Sizeof(query)),
		uintptr(unsafe.Pointer(&descBuf[0])),
		uintptr(len(descBuf)),
		uintptr(unsafe.Pointer(&bytesReturned)),
		0,
	)

	if ret != 0 {
		desc := (*STORAGE_DEVICE_DESCRIPTOR)(unsafe.Pointer(&descBuf[0]))

		// Extract strings from descriptor
		if desc.VendorIdOffset > 0 && desc.VendorIdOffset < uint32(len(descBuf)) {
			disk.Vendor = extractString(descBuf, desc.VendorIdOffset)
		}
		if desc.ProductIdOffset > 0 && desc.ProductIdOffset < uint32(len(descBuf)) {
			disk.Model = extractString(descBuf, desc.ProductIdOffset)
		}
		if desc.SerialNumberOffset > 0 && desc.SerialNumberOffset < uint32(len(descBuf)) {
			disk.Serial = extractString(descBuf, desc.SerialNumberOffset)
		}

		disk.Removable = desc.RemovableMedia != 0

		// Map bus type to transport
		switch desc.BusType {
		case BusTypeUsb:
			disk.Transport = TransportUSB
			disk.IsUSB = true
		case BusTypeNvme:
			disk.Transport = TransportNVMe
			disk.DriveType = DriveTypeSSD
		case BusTypeSata, BusTypeAta:
			disk.Transport = TransportSATA
		case BusTypeSd, BusTypeMmc:
			disk.Transport = TransportMMC
			disk.Removable = true
		default:
			disk.Transport = TransportUnknown
		}
	}

	// Trim strings
	disk.Vendor = strings.TrimSpace(disk.Vendor)
	disk.Model = strings.TrimSpace(disk.Model)
	disk.Serial = strings.TrimSpace(disk.Serial)

	// Try to detect SSD from model name if not NVMe
	if disk.DriveType == "" {
		modelLower := strings.ToLower(disk.Model)
		if strings.Contains(modelLower, "ssd") || strings.Contains(modelLower, "solid") {
			disk.DriveType = DriveTypeSSD
		} else {
			disk.DriveType = DriveTypeUnknown
		}
	}

	return disk, nil
}

// extractString extracts a null-terminated string from a buffer at the given offset.
func extractString(buf []byte, offset uint32) string {
	if offset >= uint32(len(buf)) {
		return ""
	}
	end := offset
	for end < uint32(len(buf)) && buf[end] != 0 {
		end++
	}
	return string(buf[offset:end])
}

// EnumerateFromPath is a stub for Windows (not supported).
func EnumerateFromPath(sysBlockPath string) ([]*Disk, error) {
	return EnumerateWin32()
}
