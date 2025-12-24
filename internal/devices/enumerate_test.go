package devices

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsVirtualDevice(t *testing.T) {
	tests := []struct {
		name     string
		device   string
		expected bool
	}{
		{"loop device", "loop0", true},
		{"loop device numbered", "loop12", true},
		{"ram disk", "ram0", true},
		{"device mapper", "dm-0", true},
		{"sr (optical)", "sr0", true},
		{"floppy", "fd0", true},
		{"md raid", "md0", true},
		{"zram", "zram0", true},
		{"sda", "sda", false},
		{"sdb", "sdb", false},
		{"nvme0n1", "nvme0n1", false},
		{"mmcblk0", "mmcblk0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVirtualDevice(tt.device)
			if result != tt.expected {
				t.Errorf("isVirtualDevice(%q) = %v, want %v", tt.device, result, tt.expected)
			}
		})
	}
}

func TestDetectTransport(t *testing.T) {
	tests := []struct {
		name     string
		devName  string
		expected Transport
	}{
		{"NVMe device", "nvme0n1", TransportNVMe},
		{"NVMe second controller", "nvme1n1", TransportNVMe},
		{"MMC device", "mmcblk0", TransportMMC},
		{"MMC second device", "mmcblk1", TransportMMC},
		// SATA and USB detection require sysfs path checking
		{"Unknown SATA style", "sda", TransportUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectTransport("", tt.devName)
			if result != tt.expected {
				t.Errorf("detectTransport(%q) = %v, want %v", tt.devName, result, tt.expected)
			}
		})
	}
}

func TestDiskSizeGB(t *testing.T) {
	tests := []struct {
		name      string
		sizeBytes int64
		expected  float64
	}{
		{"500GB drive", 500 * 1e9, 500.0},
		{"1TB drive", 1000 * 1e9, 1000.0},
		{"2TB drive (WD style)", 2000398934016, 2000.398934016},
		{"256GB drive", 256 * 1e9, 256.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Disk{SizeBytes: tt.sizeBytes}
			result := d.SizeGB()
			// Allow small floating point differences
			diff := result - tt.expected
			if diff < -0.001 || diff > 0.001 {
				t.Errorf("SizeGB() = %f, want %f", result, tt.expected)
			}
		})
	}
}

func TestDiskDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		disk     Disk
		expected string
	}{
		{
			name: "With model",
			disk: Disk{
				Model: "Samsung SSD 970 EVO Plus",
				Name:  "nvme0n1",
			},
			expected: "Samsung SSD 970 EVO Plus",
		},
		{
			name: "With vendor only",
			disk: Disk{
				Vendor: "Western Digital",
				Name:   "sda",
			},
			expected: "Western Digital",
		},
		{
			name: "Name only",
			disk: Disk{
				Name: "sda",
			},
			expected: "sda",
		},
		{
			name: "USB device",
			disk: Disk{
				Model: "USB Flash Drive",
				Name:  "sdb",
				IsUSB: true,
			},
			expected: "USB Flash Drive (USB)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.disk.DisplayName()
			if result != tt.expected {
				t.Errorf("DisplayName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestPartitionSizeGB(t *testing.T) {
	p := Partition{SizeBytes: 100 * 1e9}
	result := p.SizeGB()
	if result != 100.0 {
		t.Errorf("SizeGB() = %f, want 100.0", result)
	}
}

func TestTypeGUIDToName(t *testing.T) {
	tests := []struct {
		guid     string
		expected string
	}{
		{GUIDEFISystem, "EFI System"},
		{GUIDMSReserved, "Microsoft Reserved"},
		{GUIDBasicData, "Basic Data"},
		{GUIDWindowsRecovery, "Windows Recovery"},
		{GUIDLinuxFilesystem, "Linux Filesystem"},
		{GUIDLinuxSwap, "Linux Swap"},
		{"00000000-0000-0000-0000-000000000000", "Unknown"},
		{"INVALID-GUID", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.guid, func(t *testing.T) {
			result := TypeGUIDToName(tt.guid)
			if result != tt.expected {
				t.Errorf("TypeGUIDToName(%q) = %q, want %q", tt.guid, result, tt.expected)
			}
		})
	}
}

func TestEnumerateFromPath(t *testing.T) {
	// Create a mock sysfs structure
	tmpDir := t.TempDir()

	// Create a mock NVMe device
	nvmeDir := filepath.Join(tmpDir, "nvme0n1")
	if err := os.MkdirAll(filepath.Join(nvmeDir, "queue"), 0755); err != nil {
		t.Fatalf("failed to create mock sysfs: %v", err)
	}

	// Size: 1000 sectors = 512000 bytes
	if err := os.WriteFile(filepath.Join(nvmeDir, "size"), []byte("1000\n"), 0644); err != nil {
		t.Fatalf("failed to write size: %v", err)
	}

	// Removable: no
	if err := os.WriteFile(filepath.Join(nvmeDir, "removable"), []byte("0\n"), 0644); err != nil {
		t.Fatalf("failed to write removable: %v", err)
	}

	// Rotational: 0 (SSD)
	if err := os.WriteFile(filepath.Join(nvmeDir, "queue/rotational"), []byte("0\n"), 0644); err != nil {
		t.Fatalf("failed to write rotational: %v", err)
	}

	// Create a mock loop device (should be skipped)
	loopDir := filepath.Join(tmpDir, "loop0")
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatalf("failed to create mock loop: %v", err)
	}

	disks, err := EnumerateFromPath(tmpDir)
	if err != nil {
		t.Fatalf("EnumerateFromPath failed: %v", err)
	}

	// Should find exactly one disk (nvme0n1, not loop0)
	if len(disks) != 1 {
		t.Errorf("expected 1 disk, got %d", len(disks))
	}

	if len(disks) > 0 {
		disk := disks[0]
		if disk.Name != "nvme0n1" {
			t.Errorf("expected nvme0n1, got %s", disk.Name)
		}
		if disk.SizeBytes != 512000 {
			t.Errorf("expected size 512000, got %d", disk.SizeBytes)
		}
		if disk.DriveType != DriveTypeSSD {
			t.Errorf("expected SSD, got %s", disk.DriveType)
		}
		if disk.Transport != TransportNVMe {
			t.Errorf("expected NVMe transport, got %s", disk.Transport)
		}
	}
}

func TestReadSysfsFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test reading existing file
	testFile := filepath.Join(tmpDir, "test")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	content, err := readSysfsFile(tmpDir, "test")
	if err != nil {
		t.Errorf("readSysfsFile failed: %v", err)
	}
	if content != "hello world" {
		t.Errorf("got %q, want %q", content, "hello world")
	}

	// Test reading non-existent file
	_, err = readSysfsFile(tmpDir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestReadSysfsFileOrEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Test reading existing file
	testFile := filepath.Join(tmpDir, "test")
	if err := os.WriteFile(testFile, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	content := readSysfsFileOrEmpty(tmpDir, "test")
	if content != "hello" {
		t.Errorf("got %q, want %q", content, "hello")
	}

	// Test reading non-existent file
	content = readSysfsFileOrEmpty(tmpDir, "nonexistent")
	if content != "" {
		t.Errorf("got %q, want empty string", content)
	}
}
