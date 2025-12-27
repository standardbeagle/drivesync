# DriveSync

A cross-platform drive cloning utility with Windows VSS live cloning and bootable Linux USB support.

## Overview

DriveSync is a block-level drive cloning tool that works on both Windows and Linux. It provides intelligent auto-detection, live system cloning via Windows VSS, and a bootable USB option for maximum flexibility.

**Key features:**
- **Windows VSS Live Cloning**: Clone your running Windows system without rebooting
- **Smart Auto-Detection**: Automatically identifies source and destination drives
- **Bootable USB**: Self-contained Linux environment with Secure Boot support
- **GPT-aware**: Handles the "2TB to 2TB" size mismatch problem

## Quick Start

### Windows (VSS Live Clone)

**Clone your running Windows system to an external drive:**

1. Download `drivesync-windows-amd64.zip` from [releases](https://github.com/standardbeagle/drivesync/releases)
2. Extract and right-click `drivesync.exe` → **Run as Administrator**
3. Press **Enter** to clone auto-detected drives, or **S/D** to select manually
4. Clone happens live - no reboot needed!

**Use case:** Upgrade your laptop's internal SSD while Windows is running. Connect new drive via USB, clone, shutdown, swap drives.

### Linux Bootable USB - Traditional Mode

1. Download `drivesync-live-usb.zip` from [releases](https://github.com/standardbeagle/drivesync/releases)
2. Extract to a FAT32 USB stick
3. Boot from the USB (works with Secure Boot enabled)
4. Select source drive
5. Select destination drive
6. Type "clone" to confirm

### Linux Bootable USB - Self-Overwrite Mode (2 drives)

1. Extract `drivesync-live-usb.zip` to the destination drive (in USB enclosure)
2. Boot from the destination drive
3. One-click clone: internal drive → this drive
4. Swap drives, done

## Features

### Cross-Platform Support
- **Windows VSS Live Cloning** - Clone running Windows systems without reboot
- **Linux Bootable USB** - Debian Live environment with Secure Boot support
- **Smart Auto-Detection** - Automatically identifies source and destination
- **One-click workflow** - Press Enter to start clone with detected drives

### Core Capabilities
- **Block-level cloning** - Copies everything including boot sectors and partition tables
- **GPT-aware** - Handles the "2TB to 2TB" size mismatch problem
- **Partition enumeration** - Accurate space calculations, even with shrunk BitLocker volumes
- **Direct I/O** - Platform-specific unbuffered I/O for maximum performance
- **Self-overwrite mode** - Clone to the drive you booted from (Linux USB only)

### User Experience
- **Modern TUI** - Clean Bubble Tea interface with keyboard navigation
- **Progress tracking** - Real-time speed, ETA, and completion percentage
- **Error handling** - Clear messages for encryption, size mismatches, and disk issues
- **Multiple workflows** - Auto mode, manual selection, or config file

## Downloads

Get the latest release from [GitHub Releases](https://github.com/standardbeagle/drivesync/releases):

| File | Platform | Description |
|------|----------|-------------|
| `drivesync-windows-amd64.zip` | Windows | VSS live cloning - clone running systems |
| `drivesync-live-usb.zip` | Any (bootable) | Debian Live USB - Secure Boot compatible |
| `drivesync-linux-amd64` | Linux | Static binary for CLI mode |

## System Requirements

- **Windows**: Windows 10/11 with Administrator privileges
- **Bootable USB**: Any x86_64 PC with UEFI (Secure Boot works out of the box)
- **Linux**: Any modern Linux distribution with root access

## Common Use Cases

### Upgrade Your Laptop SSD (Windows)
1. Connect new SSD via USB enclosure
2. Run `drivesync.exe` as Administrator
3. Press Enter to auto-clone
4. Shutdown, swap drives, boot from new SSD

### Clone Before Hardware Failure
1. Boot from DriveSync USB
2. Clone failing drive to new drive
3. Works even if Windows won't boot

### Create Exact Backup
- Clones everything: OS, apps, files, partition layout
- Destination drive boots identically to source
- No need to reinstall or reconfigure

## Technical Details

For developers and advanced users:
- [Building from Source](docs/BUILDING.md) - Compile for Windows, Linux, or create bootable USB
- [How It Works](docs/TECHNICAL.md) - Architecture, GPT handling, and the "2TB problem"
- [Configuration](docs/CONFIGURATION.md) - KDL config files for automation

## FAQs

**Q: Can I clone to a smaller drive?**
A: Yes, if the used data fits. DriveSync calculates actual space usage and handles shrunk partitions correctly.

**Q: Will it work with BitLocker/encrypted drives?**
A: Yes. On Windows, VSS handles live encrypted volumes. Shrink the BitLocker partition first if cloning to smaller drive.

**Q: Does it work with Secure Boot enabled?**
A: Yes. The bootable USB uses Debian's signed GRUB bootloader - no need to disable Secure Boot.

**Q: How long does it take?**
A: Depends on drive size and USB speed. Typical 500GB clone over USB 3.0 takes 1-2 hours.

## License

MIT - see [LICENSE](LICENSE) file for details.

## Support

- Report issues: [GitHub Issues](https://github.com/standardbeagle/drivesync/issues)
- Discussions: [GitHub Discussions](https://github.com/standardbeagle/drivesync/discussions)
