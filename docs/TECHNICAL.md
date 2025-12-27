# Technical Documentation

Deep dive into DriveSync's architecture, algorithms, and technical challenges.

## Architecture

```
drivesync/
├── cmd/drivesync/          # Main entry point
│   └── main.go             # CLI parsing, config loading
├── internal/
│   ├── clone/              # Core cloning engine
│   │   ├── copy.go         # Block-level copy with progress
│   │   ├── open_linux.go   # O_DIRECT support (Linux)
│   │   ├── open_windows.go # FILE_FLAG_NO_BUFFERING (Windows)
│   │   └── types.go        # Size analysis and validation
│   ├── devices/            # Drive enumeration and detection
│   │   ├── enumerate.go    # Linux (sysfs + sgdisk)
│   │   ├── enumerate_windows.go  # Windows (WMI + IOCTL)
│   │   ├── boot.go         # Boot device detection
│   │   ├── match.go        # Auto-detection logic
│   │   └── types.go        # Disk and partition structs
│   ├── vss/                # Volume Shadow Copy (Windows only)
│   │   ├── vss_windows.go  # COM API wrapper for VSS
│   │   └── vss_other.go    # Stub for non-Windows
│   ├── gpt/                # GPT partition table handling
│   │   ├── gpt.go          # Parse and rewrite GPT
│   │   └── types.go        # GPT header/entry structures
│   ├── tui/                # Terminal User Interface
│   │   └── app.go          # Bubble Tea application
│   └── config/             # KDL configuration parser
│       └── config.go
├── boot/                   # Bootable USB image creation
│   └── build-debian-live.sh  # Debian Live build script
└── testdata/               # Test fixtures and mocks
```

## The 2TB Problem

### Background

Manufacturers label drives with marketing sizes (e.g., "2TB"), but actual capacity varies:

```
Western Digital Blue 2TB:     2,000,398,934,016 bytes  (1.819 TiB)
Samsung 870 EVO 2TB:          1,999,844,147,200 bytes  (1.818 TiB)
Seagate BarraCuda 2TB:        2,000,365,289,472 bytes  (1.818 TiB)
```

**Difference between WD and Samsung: ~554 MB**

Traditional cloning tools fail when:
1. Source is 2TB WD (larger)
2. Destination is 2TB Samsung (smaller)
3. Error: "destination too small"

### DriveSync Solution

DriveSync solves this with intelligent space analysis:

#### 1. **Partition Enumeration**
```go
// Linux: sgdisk -p /dev/sda
// Windows: IOCTL_DISK_GET_DRIVE_LAYOUT_EX

type Partition struct {
    Number    int
    StartLBA  int64  // First sector
    EndLBA    int64  // Last sector
    SizeBytes int64
    FSType    string
}
```

#### 2. **Calculate Last Used Byte**
```go
func (d *Disk) LastUsedByte() int64 {
    if len(d.Partitions) == 0 {
        return 0  // No partition info - can't calculate
    }

    maxLBA := int64(0)
    for _, p := range d.Partitions {
        if p.EndLBA > maxLBA {
            maxLBA = p.EndLBA
        }
    }

    return (maxLBA + 1) * int64(d.SectorSize)
}
```

#### 3. **Smart Cloning Strategy**
```go
if srcSize <= dstSize {
    // Clone entire disk (simple case)
    cloneBytes = srcSize

} else if srcLastUsed > 0 && dstSize >= srcLastUsed {
    // Source bigger, but data fits!
    cloneBytes = srcLastUsed
    needsGPTFixup = true  // Rewrite GPT backup table

} else {
    // Cannot clone - not enough space
    return error("destination too small")
}
```

#### 4. **GPT Fixup**

When cloning to smaller disk, the GPT backup table must be rewritten:

```
Source (2000 GB):
[Primary GPT] [Partitions...............] [Backup GPT at end]
                                            ^ Sector 3,906,963,455

Destination (1999 GB):
[Primary GPT] [Partitions...] [Backup GPT at new end]
                                ^ Sector 3,905,873,919
```

DriveSync rewrites the backup GPT header and table to the correct location on the destination.

## Direct I/O

### Why Direct I/O?

- **Bypasses OS cache**: No double-buffering (kernel + userspace)
- **Predictable performance**: Not affected by system memory pressure
- **Lower latency**: Direct to hardware, no cache eviction delays

### Platform-Specific Implementation

#### Linux (O_DIRECT)
```go
// open_linux.go
func openForRead(path string, directIO bool) (*os.File, error) {
    flags := os.O_RDONLY
    if directIO {
        flags |= syscall.O_DIRECT
    }
    return os.OpenFile(path, flags, 0)
}
```

Requirements:
- Buffer must be sector-aligned (4KB)
- Read/write sizes must be sector multiples

#### Windows (FILE_FLAG_NO_BUFFERING)
```go
// open_windows.go
handle, _, _ := procCreateFileW.Call(
    uintptr(unsafe.Pointer(pathPtr)),
    GENERIC_READ,
    FILE_SHARE_READ|FILE_SHARE_WRITE,
    0,
    OPEN_EXISTING,
    uintptr(FILE_FLAG_NO_BUFFERING),
    0,
)
```

Requirements:
- Same as Linux (alignment and sizing)

### Automatic Fallback

Both implementations retry with buffered I/O if direct I/O fails:
```go
f, err := os.OpenFile(path, flags|O_DIRECT, 0)
if err != nil {
    // Retry without O_DIRECT
    return os.OpenFile(path, flags, 0)
}
```

## Volume Shadow Copy Service (VSS)

### Problem: Cloning Live Windows Systems

Copying files from a running Windows system leads to:
- **Inconsistent state**: Files change during copy
- **Open files**: Cannot copy locked files (pagefile, registry hives)
- **Unbootable clone**: Inconsistent filesystem state

### Solution: VSS Snapshots

VSS creates a point-in-time snapshot of the volume:

```go
// 1. Initialize COM
ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)

// 2. Create VSS backup components
backupComp := oleutil.CreateObject("VssBackupComponents")

// 3. Start snapshot set
snapshotSetID := backupComp.StartSnapshotSet()

// 4. Add volume
snapshotID := backupComp.AddToSnapshotSet("C:\\", ...)

// 5. Create the snapshot
backupComp.PrepareForBackup()
backupComp.DoSnapshotSet()

// 6. Get snapshot device path
devicePath := backupComp.GetSnapshotProperties(snapshotID).SnapshotDeviceObject
// Example: \\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1
```

### Reading from Snapshot

The snapshot appears as a read-only block device:
```go
// Clone from snapshot instead of live disk
Clone(
    snapshot.DevicePath,  // Source: \\?\GLOBALROOT\Device\...
    destDisk.Path,        // Destination: \\.\PhysicalDrive1
    opts,
    progress,
)
```

Benefits:
- **Consistent state**: Frozen point-in-time
- **All files accessible**: No "file in use" errors
- **Bootable clone**: Filesystem consistency guaranteed

### Cleanup

```go
defer func() {
    backupComp.DeleteSnapshots(snapshotID, ...)
    backupComp.Release()
    ole.CoUninitialize()
}()
```

## Auto-Detection Algorithm

### Goal
Automatically identify source and destination when unambiguous.

### Detection Process

```go
func AutoDetectDrives(disks []*Disk) *MatchResult {
    // 1. Detect boot device
    bootDevice := findBootDevice()

    // 2. Categorize disks
    internal := []Disk  // Non-removable, not boot
    external := []Disk  // USB or removable, not boot

    // 3. Apply confidence rules
    if len(internal) == 1 && len(external) == 1 {
        // Unambiguous: exactly one of each
        return MatchResult{
            Source: internal[0],
            Destination: external[0],
            Confidence: "high",
        }
    }

    if len(internal) == 1 && len(external) > 1 {
        // Medium: one internal, multiple external
        // Choose largest external
        return MatchResult{
            Source: internal[0],
            Destination: largestExternal,
            Confidence: "medium",
        }
    }

    // Low confidence - require manual selection
    return MatchResult{Confidence: "low"}
}
```

### Confidence Levels

- **High**: Exactly one internal, one external (auto-proceed safe)
- **Medium**: One internal, multiple external (suggest with override)
- **Low**: Multiple internals or no clear pattern (require manual)

## Progress Tracking

### Atomic Progress Updates

```go
var progressUpdated int32

go func() {
    ticker := time.NewTicker(100 * time.Millisecond)
    for range ticker.C {
        atomic.StoreInt32(&progressUpdated, 1)
    }
}()

// In copy loop
if atomic.CompareAndSwapInt32(&progressUpdated, 1, 0) {
    progress <- Progress{
        Copied:    copied,
        Total:     size,
        Speed:     instantaneousSpeed,
        AvgSpeed:  averageSpeed,
        Remaining: estimatedTime,
    }
}
```

Benefits:
- **Lock-free**: No mutex overhead
- **Rate-limited**: Updates max every 100ms
- **Non-blocking**: Skips updates if channel full

## Bootable USB

### Debian Live Architecture

```
USB Drive (FAT32)
├── EFI/
│   └── BOOT/
│       ├── bootx64.efi    # Signed GRUB (Secure Boot)
│       └── grubx64.efi    # GRUB core
├── boot/
│   └── grub/
│       └── grub.cfg       # Boot menu
└── live/
    ├── vmlinuz            # Linux kernel
    ├── initrd.img         # Initial ramdisk
    └── filesystem.squashfs # Root filesystem (SquashFS)
```

### Boot Process

1. **UEFI firmware** → loads `bootx64.efi` (Debian signed GRUB)
2. **GRUB** → loads kernel + initrd from `/live`
3. **initrd** → mounts `filesystem.squashfs` as root
4. **systemd** → starts DriveSync service

### Why Debian Live?

- ✅ Secure Boot support (signed bootloader)
- ✅ Mature live-boot infrastructure
- ✅ Minimal customization needed
- ✅ Trusted by enterprises

## Performance Characteristics

### Throughput
- **USB 3.0**: ~200-400 MB/s (typical)
- **USB 3.1 Gen 2**: ~800-1000 MB/s (NVMe via USB)
- **SATA SSD → SATA SSD**: ~500 MB/s (SATA III limit)

### Memory Usage
- **Binary**: ~10 MB
- **Runtime**: ~50-100 MB
- **Buffer**: 64 KB default (configurable)

### CPU Usage
- **Minimal**: <5% during copy (I/O bound)
- **Spikes**: During GPT parsing and progress updates

## Security Considerations

### Privilege Requirements

- **Linux**: Root needed for `/dev/sd*` access
- **Windows**: Administrator for `\\.\PhysicalDrive*` and VSS

### Data Safety

- **Read-only source**: Never writes to source disk
- **Explicit confirmation**: Type "clone" to prevent accidents
- **Self-overwrite detection**: Prevents destroying boot device
- **Pre-flight checks**: Size, encryption, partition validation

### Secure Boot

The Debian Live USB uses Debian's signed GRUB bootloader:
- Chain of trust: UEFI → shim → GRUB → kernel
- No custom signing required
- Works with default Secure Boot databases

## Testing Strategy

### Unit Tests
```bash
make test
```
- Mock filesystems and block devices
- Test size calculations and GPT parsing
- Platform-agnostic logic

### Integration Tests
```bash
sudo make test-integration
```
- Real block device access
- Partition enumeration
- VSS on Windows

### Race Detection
```bash
make test-race
```
- Concurrent progress updates
- Lock-free atomic operations

## Future Enhancements

Potential improvements:
- **Parallel I/O**: Multiple reader/writer threads
- **Compression**: On-the-fly compression for smaller destinations
- **Sparse file detection**: Skip unused blocks
- **Network cloning**: Clone over network (source and destination on different machines)
- **Incremental cloning**: Only copy changed blocks
