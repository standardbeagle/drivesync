# DriveSync

A minimal UEFI-bootable drive cloning utility with a modern TUI.

## Overview

DriveSync is a standalone bootable tool for block-level drive cloning. It provides a clean, hardware-friendly alternative to Clonezilla with proper handling of the "2TB to 2TB" problem where nominally identical drives differ in actual sector count.

**Key differentiator:** Self-overwrite mode. Extract DriveSync to the destination drive, boot from it, and it clones the internal drive onto itself - running entirely from RAM while erasing its own boot media. No USB stick required. Perfect for fleet migrations: prep a stack of drives, deploy to each machine with a single click.

**User experience:** Download zip, extract to drive, boot, clone.

## Distribution

### GitHub Releases

Each release includes:

- `drivesync-linux-amd64` - Standalone binary (run from any Linux environment)
- `drivesync-usb.zip` - Bootable contents (extract to FAT32 USB stick or destination drive)
- Source tarball

### Usage Modes

**Traditional (3 drives):**
1. Extract to USB stick
2. Boot from USB
3. Clone source → destination
4. USB stick is reusable

**Self-overwrite (2 drives):**
1. Extract to destination drive (in USB enclosure)
2. Boot from destination drive
3. One-click clone internal → this drive
4. Swap drives, done
5. Destination drive is now Windows, DriveSync is gone

### Website (Docusaurus)

- Landing page with download links
- Getting started guide (both modes)
- Fleet migration guide
- UX articles:
  - "Why drive cloning tools suck" (the problem space)
  - "Understanding drive sizes" (the 2TB != 2TB issue)
  - "How DriveSync handles size mismatches"
  - "The Self-Destructing Clone Tool" (self-overwrite deep dive)

## Goals

- Boot reliably on problematic hardware (Surface Books, high-DPI displays, hybrid GPUs)
- Present drives with human-readable context, not cryptic device nodes
- Handle "2TB to 2TB" size mismatches gracefully via GPT-aware cloning
- Self-overwrite mode: boot from destination drive, clone internal → boot drive
- Enable batch fleet migrations without USB stick logistics
- Perform block-level cloning with progress and verification
- Dead simple distribution: unzip to USB, boot

## Non-Goals

- VSS/live filesystem snapshots
- Partition resizing (we handle *disk* size mismatch, not partition resize)
- Boot sector repair or BCD manipulation
- BitLocker handling
- Full filesystem-aware sparse copying (maybe Phase 2)

---

## Application Architecture

### Directory Structure

```
drivesync/
├── cmd/
│   └── drivesync/
│       └── main.go
├── internal/
│   ├── clone/
│   │   ├── copy.go
│   │   └── verify.go
│   ├── devices/
│   │   ├── enumerate.go
│   │   ├── sysfs.go
│   │   └── blkid.go
│   ├── gpt/
│   │   ├── parse.go
│   │   ├── write.go
│   │   └── types.go
│   ├── ntfs/
│   │   └── bitmap.go       # Optional: parse $Bitmap for used space
│   └── tui/
│       ├── app.go
│       ├── selection.go
│       ├── confirm.go
│       ├── sizecheck.go
│       └── progress.go
├── docs/                    # Docusaurus site
│   ├── docs/
│   ├── blog/                # UX articles
│   └── docusaurus.config.js
├── boot/
│   └── build-usb.sh         # Script to package bootable zip
├── Makefile
├── go.mod
└── README.md
```

### Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- Standard library only for everything else (no CGO)

---

## Drive Enumeration

### Data Sources

| Path | Information |
|------|-------------|
| `/sys/block/*/device/model` | Drive model name |
| `/sys/block/*/device/vendor` | Drive vendor |
| `/sys/block/*/size` | Size in 512-byte sectors |
| `/sys/block/*/removable` | 1 if removable media |
| `/sys/block/*/device/../../manufacturer` | USB manufacturer string |
| `/sys/block/*/device/../../product` | USB product string |
| `/sys/block/*/queue/rotational` | 0 for SSD, 1 for HDD |

### Partition Data

| Source | Information |
|--------|-------------|
| GPT header parsing | Partition names, type GUIDs |
| `blkid` output or direct superblock read | Filesystem type, label, UUID |

### Transport Detection

Infer from sysfs device path:
- `/sys/block/nvme*` → NVMe
- Path contains `/usb/` → USB
- Path contains `/ata/` → SATA
- Path contains `/mmcblk/` → SD/eMMC

---

## TUI Screens

### Drive Selection

```
┌─ DriveSync ─────────────────────────────────────────────────┐
│                                                             │
│  Select SOURCE drive:                                       │
│                                                             │
│  ● [nvme0n1] Samsung 970 EVO Plus              465.8 GB    │
│    ├─ nvme0n1p1: EFI System (FAT32)            100.0 MB    │
│    ├─ nvme0n1p2: Microsoft Reserved              16.0 MB    │
│    └─ nvme0n1p3: Windows 11 (NTFS)             465.2 GB    │
│                                                             │
│  ○ [sda] WD_BLACK SN850X (USB)                 931.5 GB    │
│    └─ (unpartitioned)                                       │
│                                                             │
│  ○ [sdb] SanDisk Ultra USB 3.0                  28.9 GB    │
│    └─ sdb1: DRIVESYNC (FAT32)                   28.9 GB    │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  ↑/↓ Select   Enter Confirm   Q Quit                       │
└─────────────────────────────────────────────────────────────┘
```

### Size Analysis (when destination < source)

```
┌─ Size Analysis ─────────────────────────────────────────────┐
│                                                             │
│  Source:      WD Blue 2TB              2,000.4 GB          │
│  Destination: Samsung 870 2TB          1,999.8 GB          │
│  Difference:  624.0 MB (destination smaller)               │
│                                                             │
│  ┌─ Partition Layout ───────────────────────────────────┐  │
│  │ nvme0n1p1: EFI System                      100.0 MB  │  │
│  │ nvme0n1p2: Microsoft Reserved               16.0 MB  │  │
│  │ nvme0n1p3: Windows (NTFS)               1,999.5 GB  │  │
│  │   └─ Last used sector:             1,022,458,880     │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  ✓ Clone will fit. Trailing space becomes unallocated.    │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  Enter Continue   Esc Cancel                                │
└─────────────────────────────────────────────────────────────┘
```

### Confirmation

```
┌─ Confirm Clone ─────────────────────────────────────────────┐
│                                                             │
│  SOURCE: Samsung 970 EVO Plus (465.8 GB)                   │
│          nvme0n1                                            │
│                                                             │
│  DESTINATION: WD_BLACK SN850X (931.5 GB)                   │
│               sda                                           │
│                                                             │
│  ⚠ ALL DATA ON DESTINATION WILL BE DESTROYED               │
│                                                             │
│  Type "clone" to confirm:  clone█                          │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  Esc Cancel                                                 │
└─────────────────────────────────────────────────────────────┘
```

### Progress

```
┌─ Cloning ───────────────────────────────────────────────────┐
│                                                             │
│  Samsung 970 EVO Plus  →  WD_BLACK SN850X                  │
│                                                             │
│  ████████████████████████████░░░░░░░░░░░░░░░░  62.4%       │
│                                                             │
│  Copied:     291.2 GB / 465.8 GB                           │
│  Speed:      412 MB/s                                       │
│  Elapsed:    11:47                                          │
│  Remaining:  ~7:05                                          │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  Ctrl+C Abort                                               │
└─────────────────────────────────────────────────────────────┘
```

### Completion

```
┌─ Complete ──────────────────────────────────────────────────┐
│                                                             │
│  ✓ Clone completed successfully                            │
│                                                             │
│  Copied:     465.8 GB                                       │
│  Duration:   18:52                                          │
│  Avg Speed:  411 MB/s                                       │
│  Verified:   ✓ All blocks match                            │
│                                                             │
│  It is now safe to remove the destination drive.           │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  Enter New Clone   R Reboot   P Power Off                  │
└─────────────────────────────────────────────────────────────┘
```

---

## Copy Engine

### Core Loop

```go
func Clone(src, dst *os.File, size int64, progress chan<- Progress) error {
    buf := make([]byte, 64*1024) // 64KB blocks
    var copied int64
    
    for copied < size {
        n, err := src.Read(buf)
        if err != nil && err != io.EOF {
            return fmt.Errorf("read error at offset %d: %w", copied, err)
        }
        if n == 0 {
            break
        }
        
        if _, err := dst.Write(buf[:n]); err != nil {
            return fmt.Errorf("write error at offset %d: %w", copied, err)
        }
        
        copied += int64(n)
        progress <- Progress{Copied: copied, Total: size}
    }
    
    return dst.Sync()
}
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| Block size | 64KB | Tunable for performance |
| Verify | Off | Read-back comparison after clone |
| Direct I/O | On | Bypass page cache |

---

## Large to Small Drive Handling

### The Problem

"2TB" drives vary by manufacturer:
- WD Blue 2TB: 2,000,398,934,016 bytes
- Samsung 870 2TB: 1,999,844,147,200 bytes
- Difference: 554 MB

Block-level clone fails even though actual used data is 500GB.

### Solution

1. Parse GPT to find last partition end
2. If last used sector fits on destination → clone only that extent
3. Rewrite GPT backup table at new disk end
4. Trailing space remains unallocated (Windows can extend later)

### GPT Fixup

```go
func FixupGPT(device string, newSizeBytes int64) error {
    newLastLBA := (newSizeBytes / 512) - 1
    
    // Read and update primary header
    primary := readGPTHeader(device, 1)
    primary.BackupLBA = newLastLBA
    primary.LastUsableLBA = newLastLBA - 33
    primary.HeaderCRC32 = calculateCRC(primary)
    writeGPTHeader(device, 1, primary)
    
    // Write backup at new location
    backup := primary
    backup.MyLBA = newLastLBA
    backup.BackupLBA = 1
    backup.PartitionEntryLBA = newLastLBA - 32
    backup.HeaderCRC32 = calculateCRC(backup)
    writeGPTHeader(device, newLastLBA, backup)
    
    return nil
}
```

### Failure Case

If last used sector exceeds destination capacity:
- Show clear error
- Tell user to shrink partition in Windows first
- Don't attempt partial clone

---

## Boot Environment

### Base

Use a minimal Linux live environment. Candidates:
- **Void Linux live** - musl-based, minimal, good hardware support
- **Debian live minimal** - boring and reliable
- **Arch live** - good drivers, heavier

Selection criteria: boots on Surface Books, has NVMe/USB drivers, small.

### Customization

1. Auto-login to console
2. Auto-run drivesync binary
3. Include terminus-font for readable console

### Build Script

```bash
#!/bin/bash
# boot/build-usb.sh

# Download base image
# Mount, inject binary to /usr/local/bin
# Add systemd unit or init.d script for autostart
# Repack as zip

# Output: dist/drivesync-usb.zip
```

The details depend on chosen distro. This is packaging, not core functionality.

---

## Build & Release

### Makefile

```makefile
VERSION := $(shell git describe --tags --always)

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-s -w -X main.version=$(VERSION)" \
		-o dist/drivesync-linux-amd64 \
		./cmd/drivesync

usb: build
	./boot/build-usb.sh

docs:
	cd docs && npm run build

release: build usb
	# Creates GitHub release with binary + zip

clean:
	rm -rf dist/
```

### GitHub Actions

- Build on tag push
- Run tests
- Create release with artifacts
- Deploy Docusaurus to GitHub Pages

---

## Self-Overwrite Mode

DriveSync can clone the internal drive onto the drive it booted from, overwriting itself during the clone. This eliminates the need for a third drive (USB boot stick) and enables two key workflows:

### Use Case 1: Remote Family Tech Support

Your mom's laptop is slow and needs a bigger SSD. You're 1,000 miles away.

1. **You:** Buy new SSD, format it, extract DriveSync
2. **You:** Ship drive to mom (or prep via TeamViewer if she has enclosure)
3. **Phone call:** "Plug in the drive. Restart. When you see the screen, press Enter."
4. **Wait:** Clone runs, she sees progress bar
5. **Phone call:** "It says done. Now shut down and swap the drives."
6. **Done:** She ships old drive back, or keeps it as backup

No explaining BIOS boot order. No "which /dev/sda is which." No walking through Clonezilla menus. One button.

### Use Case 2: Fleet Migration

IT department needs to migrate 50 laptops from 500GB to 1TB drives:

1. **Prep station:** One technician formats destination drives, extracts DriveSync to each
2. **Field work:** Technicians visit each laptop, swap in prepared drive, boot, one-click clone
3. **Completion:** Swap drives back, laptop boots from new drive with cloned Windows

No USB sticks to track. Each destination drive is its own bootable clone tool.

### Workflow

```
┌─────────────────────────────────────────────────────────────┐
│                     Prep Station                            │
├─────────────────────────────────────────────────────────────┤
│  1. Format new drive as FAT32 (GPT, single partition)      │
│  2. Extract drivesync-usb.zip to root                       │
│  3. Repeat for all destination drives                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     At Each Laptop                          │
├─────────────────────────────────────────────────────────────┤
│  1. Connect prepared drive via USB                          │
│  2. Boot from USB                                           │
│  3. DriveSync auto-detects internal Windows                 │
│  4. One-click clone (overwrites boot drive)                 │
│  5. Shutdown, swap drives physically                        │
│  6. Boot - Windows on new drive                             │
└─────────────────────────────────────────────────────────────┘
```

### Detection Logic

```go
type BootContext struct {
    BootDevice    *Disk   // Drive we booted from
    InternalDrive *Disk   // Detected Windows installation  
    CanSelfOverwrite bool
}

func DetectBootContext() (*BootContext, error) {
    // Find our boot device
    bootDev := findBootDevice()  // /proc/cmdline, /sys/firmware/efi
    
    // Is boot device USB/removable?
    bootDisk := getDisk(bootDev)
    if !bootDisk.IsUSB && !bootDisk.IsRemovable {
        return &BootContext{CanSelfOverwrite: false}, nil
    }
    
    // Find internal Windows installation
    for _, disk := range getAllDisks() {
        if disk.Path == bootDev {
            continue
        }
        if disk.IsUSB || disk.IsRemovable {
            continue
        }
        if hasWindowsInstallation(disk) {
            return &BootContext{
                BootDevice:       bootDisk,
                InternalDrive:    disk,
                CanSelfOverwrite: true,
            }, nil
        }
    }
    
    return &BootContext{CanSelfOverwrite: false}, nil
}

func findBootDevice() string {
    // Method 1: EFI boot partition from sysfs
    // /sys/firmware/efi/efivars/LoaderDevicePartUUID-*
    
    // Method 2: Parse /proc/cmdline for root= or BOOT_IMAGE=
    
    // Method 3: Find mounted /boot/efi, trace to device
}

func hasWindowsInstallation(d *Disk) bool {
    // Check for:
    // 1. EFI System Partition with /EFI/Microsoft/Boot/bootmgfw.efi
    // 2. NTFS partition with Windows directory
    // Can check NTFS MFT without mounting
}
```

### TUI: Self-Overwrite Mode

When self-overwrite is detected, show simplified one-click interface:

```
┌─ DriveSync ─────────────────────────────────────────────────┐
│                                                             │
│  Ready to clone                                             │
│                                                             │
│  FROM (internal):                                           │
│    [nvme0n1] Samsung 970 EVO Plus              465.8 GB    │
│    └─ Windows 11 Pro                                        │
│    └─ 234.2 GB used                                         │
│                                                             │
│  TO (this drive):                                           │
│    [sda] WD_BLACK SN850X (USB)                 931.5 GB    │
│    └─ Will be completely overwritten                        │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              >>> Start Clone <<<                      │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  DriveSync will continue running from memory.               │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│  Enter Clone   M Manual Mode   Q Quit                       │
└─────────────────────────────────────────────────────────────┘
```

### Technical Requirements

**Boot to RAM:**

The kernel and initramfs must be fully loaded into RAM before DriveSync starts. The boot device must be unmountable.

```bash
# In boot config (syslinux.cfg / grub.cfg)
# Add toram parameter
APPEND initrd=/initramfs.cpio.gz toram quiet
```

**Release boot device:**

```go
func PrepareForSelfOverwrite(bootDev string) error {
    // Sync all filesystems
    syscall.Sync()
    
    // Unmount boot device and all its partitions
    mounts := parseMounts()
    for _, m := range mounts {
        if strings.HasPrefix(m.Device, bootDev) {
            if err := syscall.Unmount(m.Path, 0); err != nil {
                return err
            }
        }
    }
    
    // Drop caches to ensure nothing references the device
    os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0644)
    
    return nil
}
```

**Progress during self-overwrite:**

Once we start writing, we can't read DriveSync from disk. But we're already in RAM, so the TUI keeps running normally. The only difference is we can't recover if the process dies - but that's true of any clone operation.

### Batch Prep Script

For IT departments preparing many drives:

```bash
#!/bin/bash
# prep-migration-drives.sh
# Format and prepare multiple drives for migration

DRIVESYNC_ZIP=$1

if [ -z "$DRIVESYNC_ZIP" ]; then
    echo "Usage: $0 <drivesync-usb.zip>"
    exit 1
fi

echo "Insert drives one at a time. Press Enter after each."
echo "Press Ctrl+C when done."

COUNT=0
while true; do
    read -p "Insert drive $((COUNT+1)) and press Enter: "
    
    # Find newly inserted drive (most recent in dmesg)
    DRIVE=$(dmesg | grep -o 'sd[a-z]' | tail -1)
    
    if [ -z "$DRIVE" ]; then
        echo "No new drive detected"
        continue
    fi
    
    echo "Preparing /dev/$DRIVE..."
    
    # Create GPT with single FAT32 partition
    parted /dev/$DRIVE --script mklabel gpt
    parted /dev/$DRIVE --script mkpart primary fat32 1MiB 100%
    parted /dev/$DRIVE --script set 1 boot on
    
    # Format
    mkfs.fat -F32 -n "DRIVESYNC" /dev/${DRIVE}1
    
    # Mount and extract
    mkdir -p /tmp/prep
    mount /dev/${DRIVE}1 /tmp/prep
    unzip -o $DRIVESYNC_ZIP -d /tmp/prep
    umount /tmp/prep
    
    echo "Drive $((++COUNT)) ready. Remove and insert next."
done
```

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| Multiple internal drives | Show picker with Windows installations highlighted |
| Multiple Windows installations | Show all, let user pick |
| No Windows found | Offer manual mode or show all internal drives |
| Boot device is internal NVMe | Standard mode (not self-overwrite) |
| USB drive too small | Show size analysis screen, error if won't fit |
| Clone fails mid-write | Drive is toast - but so is any failed clone |

### Test Scenarios

| Test | Setup | Expected |
|------|-------|----------|
| Happy path | Boot from USB, internal has Windows | Auto-detect, one-click clone |
| Size mismatch | USB smaller than internal | Size analysis, clone if fits |
| No Windows | Internal has Linux | Fall back to manual mode |
| Internal is USB | Booted from internal NVMe | Standard mode, no self-overwrite |
| Multiple Windows | Dual boot system | Show picker |

---

## Test Fixtures: Realistic Drive Contents

Real Windows drives have predictable structure. Test fixtures should mirror this.

### Typical Windows 11 GPT Layout

| Partition | Type GUID | Size | Filesystem | Contents |
|-----------|-----------|------|------------|----------|
| 1 | EFI System (C12A7328-F81F-11D2-BA4B-00A0C93EC93B) | 100-260 MB | FAT32 | `/EFI/Microsoft/Boot/bootmgfw.efi`, `/EFI/Boot/bootx64.efi` |
| 2 | Microsoft Reserved (E3C9E316-0B5C-4DB8-817D-F92DF00215AE) | 16 MB | None | Empty, reserved for Windows |
| 3 | Basic Data (EBC0A0A2-B9E5-4433-87C0-68B6B72699C7) | Remainder | NTFS | Windows installation (C:) |
| 4 | Windows Recovery (DE94BBA4-06D1-4D40-A16A-BFD50179D6AC) | 500-1000 MB | NTFS | WinRE image |

### OEM Variations

Dell/HP/Lenovo often add:
- OEM diagnostic partition (FAT32, 100-500 MB)
- Recovery image partition (NTFS, 10-20 GB)
- Push-button reset partition

### Boot Configuration

EFI System Partition contains:
```
/EFI/
├── Boot/
│   └── bootx64.efi          # Fallback bootloader
├── Microsoft/
│   └── Boot/
│       ├── bootmgfw.efi     # Windows Boot Manager
│       ├── BCD              # Boot Configuration Data
│       └── memtest.efi
└── [OEM]/                   # Vendor recovery tools
```

### Realistic Fill Patterns

| Partition | Typical Used % | Notes |
|-----------|----------------|-------|
| EFI | 5-10% | Small files, mostly empty |
| MSR | 0% | Always empty |
| Windows (C:) | 30-80% | Varies wildly by user |
| Recovery | 90%+ | Compressed WIM image |

### Test Fixture Generator

```bash
#!/bin/bash
# Create a realistic Windows-like test disk image

IMG=$1
SIZE_MB=${2:-50000}  # 50GB default

# Create sparse image
truncate -s ${SIZE_MB}M $IMG

# Partition with gdisk/sgdisk
sgdisk --clear \
  --new=1:2048:+260M --typecode=1:ef00 --change-name=1:"EFI System" \
  --new=2:0:+16M --typecode=2:0c01 --change-name=2:"Microsoft Reserved" \
  --new=3:0:+$((SIZE_MB - 1000))M --typecode=3:0700 --change-name=3:"Windows" \
  --new=4:0:+500M --typecode=4:2700 --change-name=4:"Recovery" \
  $IMG

# Setup loop device with partitions
LOOP=$(losetup --find --show --partscan $IMG)

# Format partitions
mkfs.fat -F32 -n "SYSTEM" ${LOOP}p1
mkfs.ntfs -f -L "Windows" ${LOOP}p3
mkfs.ntfs -f -L "Recovery" ${LOOP}p4

# Populate EFI structure
mkdir -p /tmp/efi
mount ${LOOP}p1 /tmp/efi
mkdir -p /tmp/efi/EFI/Boot
mkdir -p /tmp/efi/EFI/Microsoft/Boot
echo "placeholder" > /tmp/efi/EFI/Boot/bootx64.efi
echo "placeholder" > /tmp/efi/EFI/Microsoft/Boot/bootmgfw.efi
umount /tmp/efi

# Add some realistic fill to Windows partition
mount ${LOOP}p3 /tmp/win
# Create sparse files to simulate used space
dd if=/dev/zero of=/tmp/win/pagefile.sys bs=1M count=4096 conv=sparse
dd if=/dev/zero of=/tmp/win/hiberfil.sys bs=1M count=3072 conv=sparse
mkdir -p /tmp/win/Windows/System32
dd if=/dev/urandom of=/tmp/win/Windows/System32/ntoskrnl.exe bs=1M count=12
umount /tmp/win

losetup -d $LOOP
echo "Created test image: $IMG"
```

### Test Scenarios

| Scenario | Source | Destination | Expected Behavior |
|----------|--------|-------------|-------------------|
| Happy path | 500GB, 200GB used | 1TB | Clone, no adjustment needed |
| Exact same size | 2TB (2000.4GB) | 2TB (2000.4GB) | Clone, GPT backup in place |
| Slightly smaller | 2TB (2000.4GB) | 2TB (1999.8GB) | Clone to last used sector, rewrite GPT |
| Much smaller, fits | 2TB, 400GB used | 500GB | Clone to last used sector, rewrite GPT |
| Much smaller, doesn't fit | 2TB, 600GB used | 500GB | Error: must shrink partition first |
| Unpartitioned dest | 500GB GPT | 1TB raw | Clone, GPT copied naturally |
| MBR source | 500GB MBR | 1TB | Error or warning: MBR not supported |
| Self-overwrite | Internal 500GB | Boot USB 1TB | Detect, offer one-click, clone from RAM |
| Self-overwrite too small | Internal 1TB, 600GB used | Boot USB 500GB | Error: destination too small |

---

## Testing Strategy

Block device operations can't be safely tested in Docker. Use layered approach with increasing fidelity.

### Layer 1: Unit Tests (Go tests, CI)

No privileges needed. Test pure logic:

```go
// gpt/parse_test.go
func TestParseGPTHeader(t *testing.T) {
    // Read from testdata/fixtures/gpt_header.bin
    header, err := ParseHeader(testFixture)
    assert.Equal(t, header.Signature, "EFI PART")
    assert.Equal(t, header.DiskGUID, expectedGUID)
}

// devices/sysfs_test.go  
func TestParseSysfsModel(t *testing.T) {
    // Mock filesystem or use testdata/
    model := parseModelFile("testdata/sysfs/nvme0n1/device/model")
    assert.Equal(t, "Samsung SSD 970 EVO Plus", model)
}

// clone/size_test.go
func TestCalculateCloneExtent(t *testing.T) {
    src := DiskInfo{Size: 2000398934016, LastUsedLBA: 1953525134}
    dst := DiskInfo{Size: 1999844147200}
    
    extent, err := CalculateCloneExtent(src, dst)
    assert.NoError(t, err)
    assert.Less(t, extent.EndLBA, dst.Size/512)
}
```

### Layer 2: Loop Device Integration (CI with sudo)

Test actual block operations on virtual devices:

```go
// clone/copy_integration_test.go
// +build integration

func TestCloneWithLoopDevices(t *testing.T) {
    src := createTestImage(t, "500M", withWindowsPartitions)
    dst := createTestImage(t, "498M", empty)
    defer cleanup(src, dst)
    
    srcLoop := attachLoop(t, src)
    dstLoop := attachLoop(t, dst)
    
    err := Clone(srcLoop, dstLoop, CloneOptions{})
    assert.NoError(t, err)
    
    // Verify GPT was fixed up
    dstGPT := readGPT(dstLoop)
    assert.Equal(t, dstGPT.BackupLBA, (498*1024*1024/512)-1)
}
```

GitHub Actions config:
```yaml
jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup test images
        run: |
          ./scripts/create-test-fixtures.sh
          
      - name: Run integration tests
        run: |
          sudo go test -tags=integration ./...
```

### Layer 3: QEMU System Tests (nightly/pre-release)

Full boot-to-completion testing:

```bash
#!/bin/bash
# test/qemu/run-clone-test.sh

# Create test drives
qemu-img create -f raw source.img 2000M
qemu-img create -f raw dest.img 1998M

# Populate source with Windows-like structure
./scripts/create-test-fixtures.sh source.img windows

# Boot DriveSync image with expect script
qemu-system-x86_64 \
  -enable-kvm \
  -m 1G \
  -nographic \
  -drive file=dist/drivesync-usb.img,format=raw,if=virtio \
  -drive file=source.img,format=raw,if=virtio \
  -drive file=dest.img,format=raw,if=virtio \
  | expect test/qemu/clone-flow.expect

# Verify result
./scripts/verify-clone.sh source.img dest.img
```

Self-overwrite mode test:

```bash
#!/bin/bash
# test/qemu/run-self-overwrite-test.sh

# Create internal drive with Windows
qemu-img create -f raw internal.img 500M
./scripts/create-test-fixtures.sh internal.img windows

# Create destination drive with DriveSync installed
qemu-img create -f raw dest-boot.img 1000M
./scripts/create-bootable-dest.sh dest-boot.img

# Boot from destination drive, clone internal onto it
qemu-system-x86_64 \
  -enable-kvm \
  -m 1G \
  -nographic \
  -drive file=dest-boot.img,format=raw,if=virtio \
  -drive file=internal.img,format=raw,if=virtio \
  | expect test/qemu/self-overwrite-flow.expect

# Verify: dest-boot.img should now contain Windows, not DriveSync
./scripts/verify-clone.sh internal.img dest-boot.img
./scripts/verify-not-drivesync.sh dest-boot.img
```

Expect script for TUI automation:
```expect
#!/usr/bin/expect
# test/qemu/clone-flow.expect

set timeout 120

# Wait for drive selection
expect "Select SOURCE drive"
send "\r"  ;# Select first drive

expect "Select DESTINATION drive"  
send "\033\[B"  ;# Down arrow
send "\r"

expect "Type \"clone\" to confirm"
send "clone\r"

expect "Clone completed successfully"
send "p"  ;# Power off
```

Self-overwrite expect script:
```expect
#!/usr/bin/expect
# test/qemu/self-overwrite-flow.expect

set timeout 120

# Should auto-detect self-overwrite mode
expect "Ready to clone"
expect "FROM (internal):"
expect "TO (this drive):"

# One-click clone
expect "Start Clone"
send "\r"

expect "Clone completed successfully"
send "p"  ;# Power off
```

### Layer 4: Fault Injection (dm-flakey, scsi_debug)

Test error handling and recovery:

```bash
#!/bin/bash
# test/fault/test-read-errors.sh

# Create source image
dd if=/dev/zero of=source.img bs=1M count=500
losetup /dev/loop0 source.img

# Create flakey device that fails reads at specific offset
# Good for 100MB, then fails for 1MB, then good again
echo "0 204800 linear /dev/loop0 0
204800 2048 error
206848 817152 linear /dev/loop0 206848" | dmsetup create flakey-source

# Run clone, expect graceful failure
./drivesync --source /dev/mapper/flakey-source --dest /dev/loop1 --yes
EXIT_CODE=$?

# Should exit non-zero with clear error message
[ $EXIT_CODE -ne 0 ] || fail "Should have failed on read error"
```

scsi_debug for hardware variance:
```bash
# Test 4K sector drive
modprobe scsi_debug dev_size_mb=1000 sector_size=4096
# Creates /dev/sdX with 4096-byte sectors

# Test different device identification strings
modprobe scsi_debug dev_size_mb=1000 \
  vendor="WD" \
  product="BLACK SN850X" \
  revision="1.0"
```

### Test Matrix

| Category | Tool | Runs | Tests |
|----------|------|------|-------|
| Unit | `go test` | Every commit | GPT parsing, sysfs parsing, size math |
| Integration | Loop devices | Every PR | Clone operations, GPT fixup |
| System | QEMU | Nightly | Full boot, TUI flow, device enumeration |
| Self-overwrite | QEMU | Nightly | Boot from dest, detect internal, clone, verify |
| Fault | dm-flakey | Weekly | Read errors, write errors, disconnects |
| Hardware | scsi_debug | Weekly | 4K sectors, varied device strings |
| Visual | QEMU + VNC | Pre-release | TUI rendering, accessibility |

### CI Configuration

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test ./...

  integration:
    runs-on: ubuntu-latest
    needs: unit
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - name: Create test fixtures
        run: sudo ./scripts/create-test-fixtures.sh
      - name: Integration tests
        run: sudo go test -tags=integration -v ./...

  qemu:
    runs-on: ubuntu-latest
    needs: integration
    if: github.event_name == 'schedule' || github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - name: Install QEMU
        run: sudo apt-get install -y qemu-system-x86 qemu-utils expect
      - name: Build USB image
        run: make usb
      - name: Run QEMU tests
        run: ./test/qemu/run-all.sh
```

### Makefile Targets

```makefile
test:
	go test ./...

test-integration:
	sudo go test -tags=integration ./...

test-qemu: usb
	./test/qemu/run-all.sh

test-fault:
	sudo ./test/fault/run-all.sh

test-all: test test-integration test-qemu test-fault

fixtures:
	./scripts/create-test-fixtures.sh testdata/
```

---

## Future Enhancements

### Phase 2
- NTFS $Bitmap parsing for true used-space detection
- Sparse cloning (skip unused blocks)
- Clone to/from image file

### Phase 3  
- Network share support (SMB/NFS targets)
- PXE boot option
- Basic partition table editor

---

## References

- [GPT Specification (UEFI)](https://uefi.org/specifications)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
