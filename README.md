# DriveSync

A minimal UEFI-bootable drive cloning utility with a modern TUI.

## Overview

DriveSync is a standalone bootable tool for block-level drive cloning. It provides a clean, hardware-friendly alternative to Clonezilla with proper handling of the "2TB to 2TB" problem where nominally identical drives differ in actual sector count.

**Key differentiator:** Self-overwrite mode. Extract DriveSync to the destination drive, boot from it, and it clones the internal drive onto itself - running entirely from RAM while erasing its own boot media. No USB stick required.

## Quick Start

### Traditional Mode (3 drives)

1. Extract `drivesync-usb.zip` to a FAT32 USB stick
2. Boot from the USB
3. Select source drive
4. Select destination drive
5. Type "clone" to confirm

### Self-Overwrite Mode (2 drives)

1. Extract `drivesync-usb.zip` to the destination drive (in USB enclosure)
2. Boot from the destination drive
3. One-click clone: internal drive → this drive
4. Swap drives, done

## Features

- **Modern TUI** - Clean interface with keyboard navigation
- **Block-level cloning** - Copies everything including boot sectors
- **GPT-aware** - Handles the "2TB to 2TB" size mismatch problem
- **Self-overwrite mode** - Clone to the drive you booted from
- **Progress tracking** - Real-time speed and ETA
- **Verification** - Optional read-back verification

## Building

```bash
# Build Linux binary
make build

# Run tests
make test

# Create bootable USB image
make usb
```

## Requirements

- Linux (for building and running)
- Go 1.22+ (for building)
- Root privileges (for running)

## Architecture

```
drivesync/
├── cmd/drivesync/          # Main entry point
├── internal/
│   ├── clone/              # Copy engine
│   ├── devices/            # Drive enumeration
│   ├── gpt/                # GPT parsing/writing
│   └── tui/                # Bubble Tea UI
├── boot/                   # Bootable image scripts
└── testdata/               # Test fixtures
```

## The 2TB Problem

"2TB" drives vary by manufacturer:
- WD Blue 2TB: 2,000,398,934,016 bytes
- Samsung 870 2TB: 1,999,844,147,200 bytes
- Difference: ~554 MB

DriveSync solves this by:
1. Parsing GPT to find the last used sector
2. If data fits on destination → clone only used extent
3. Rewrite GPT backup table at new disk end

## License

MIT
