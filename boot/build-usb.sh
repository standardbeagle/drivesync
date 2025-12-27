#!/bin/bash
#
# DriveSync Bootable USB Image Builder
#
# This script creates bootable USB images containing DriveSync.
# The image uses a minimal Linux environment that boots directly into DriveSync.
#
# Requirements:
#   - Go binary already built (run 'make build' first)
#   - zip utility
#
# Output:
#   dist/drivesync-usb.zip      - Standard interactive mode
#   dist/drivesync-usb-auto.zip - Auto-clone mode (no interaction required)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DIST_DIR="$PROJECT_DIR/dist"
WORK_DIR="$DIST_DIR/usb-build"
BINARY="$DIST_DIR/drivesync-linux-amd64"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

check_prerequisites() {
    info "Checking prerequisites..."

    if [ ! -f "$BINARY" ]; then
        error "Binary not found: $BINARY\nRun 'make build' first."
    fi

    # Check for required tools
    local required_tools="zip"
    for tool in $required_tools; do
        if ! command -v "$tool" &> /dev/null; then
            error "Required tool not found: $tool"
        fi
    done

    info "Prerequisites OK"
}

create_base_structure() {
    local work_dir="$1"

    info "Creating base bootable structure in $work_dir..."

    # Clean up any previous build
    rm -rf "$work_dir"
    mkdir -p "$work_dir"

    # Create EFI boot structure
    mkdir -p "$work_dir/EFI/BOOT"
    mkdir -p "$work_dir/boot"

    # Copy DriveSync binary
    cp "$BINARY" "$work_dir/drivesync"
    chmod +x "$work_dir/drivesync"

    # Create a startup script
    cat > "$work_dir/start-drivesync.sh" << 'EOF'
#!/bin/sh
# DriveSync Startup Script
# This script is called by the init system to start DriveSync

# Wait for devices to settle
sleep 2

# Clear screen
clear

# Run DriveSync
exec /drivesync
EOF
    chmod +x "$work_dir/start-drivesync.sh"
}

create_standard_readme() {
    local work_dir="$1"

    cat > "$work_dir/README.txt" << 'EOF'
DriveSync Bootable USB - Standard Edition

QUICK START:
1. Restart your computer
2. Enter BIOS/UEFI boot menu (usually F12, F2, or Del at startup)
3. Select this USB drive
4. DriveSync will start automatically
5. Select source and destination drives
6. Type "clone" to confirm

SELF-OVERWRITE MODE:
If you extracted DriveSync to the destination drive instead of a USB stick,
DriveSync will detect this and offer one-click cloning of your internal drive.

For more information: https://github.com/standardbeagle/drivesync
EOF
}

create_auto_config() {
    local work_dir="$1"

    # Create auto-clone configuration
    cat > "$work_dir/drivesync.kdl" << 'EOF'
// DriveSync Auto-Clone Configuration
// This configuration automatically clones the internal drive to this boot drive

// Auto mode - no user interaction required
mode "auto"

// Source: First internal drive with Windows (or first non-USB drive)
source "internal"

// Destination: The drive we booted from (this USB/drive)
destination "boot-drive"

// Verify the clone with read-back comparison
verify false

// After successful clone, show completion screen
on-complete "prompt"
EOF
}

create_auto_readme() {
    local work_dir="$1"

    cat > "$work_dir/README.txt" << 'EOF'
DriveSync Bootable USB - Auto-Clone Edition

!!! AUTOMATIC CLONING - NO CONFIRMATION REQUIRED !!!

This version will AUTOMATICALLY clone the internal drive to itself
when booted. No user interaction is needed.

HOW IT WORKS:
1. Boot from this drive
2. DriveSync detects the internal drive (source)
3. DriveSync uses this boot drive as destination
4. Cloning starts IMMEDIATELY
5. Shows completion screen when done

USE CASE:
- Prepare destination drive with DriveSync
- Insert into target machine
- Boot from drive
- Walk away - clone happens automatically
- Return when complete

WARNING:
This will OVERWRITE this entire drive with the contents of the
internal drive. Make sure this is what you want!

CONFIGURATION:
Edit drivesync.kdl to customize behavior:
- Change "on-complete" to "shutdown" for auto power-off
- Change "verify" to "true" for read-back verification

For more information: https://github.com/standardbeagle/drivesync
EOF
}

create_syslinux_config() {
    local work_dir="$1"

    mkdir -p "$work_dir/syslinux"

    # SYSLINUX config for BIOS boot
    cat > "$work_dir/syslinux/syslinux.cfg" << 'EOF'
DEFAULT drivesync
TIMEOUT 30
PROMPT 0

LABEL drivesync
    MENU LABEL DriveSync
    LINUX /boot/vmlinuz
    INITRD /boot/initramfs.cpio.gz
    APPEND toram quiet loglevel=3
EOF

    # Copy config for both locations
    cp "$work_dir/syslinux/syslinux.cfg" "$work_dir/syslinux.cfg"
}

create_efi_stub() {
    local work_dir="$1"

    # Create a simple EFI startup script
    # In production, this would be a proper EFI application or GRUB
    cat > "$work_dir/EFI/BOOT/startup.nsh" << 'EOF'
@echo -off
echo DriveSync Boot
fs0:
cd \boot
vmlinuz initrd=\boot\initramfs.cpio.gz toram quiet
EOF
}

create_initramfs_placeholder() {
    local work_dir="$1"

    # Create placeholder instructions
    cat > "$work_dir/boot/BUILD-INSTRUCTIONS.txt" << 'EOF'
BUILDING A COMPLETE BOOTABLE IMAGE
===================================

To create a fully bootable DriveSync USB, you need:

1. A Linux kernel (vmlinuz)
2. An initramfs containing:
   - BusyBox or toybox for basic utilities
   - Device drivers (NVMe, USB, SATA)
   - The DriveSync binary
   - An init script that runs DriveSync

RECOMMENDED:
Use the Debian Live build script instead:
  make usb-live

This creates a complete bootable image with:
- Full Debian Linux with all drivers
- Secure Boot compatible (signed GRUB)
- Works on Surface Pro and modern PCs

CUSTOM INITRAMFS:
See scripts/build-initramfs.sh for a complete example.

The resulting files should be:
- boot/vmlinuz      (Linux kernel)
- boot/initramfs.cpio.gz  (Initial RAM filesystem)
EOF
}

package_zip() {
    local work_dir="$1"
    local output_file="$2"

    info "Creating ZIP package: $output_file..."

    cd "$work_dir"
    zip -rq "$output_file" .

    info "Created: $output_file"
}

build_standard() {
    local work_dir="$DIST_DIR/usb-build-standard"

    info "Building standard edition..."
    create_base_structure "$work_dir"
    create_standard_readme "$work_dir"
    create_syslinux_config "$work_dir"
    create_efi_stub "$work_dir"
    create_initramfs_placeholder "$work_dir"
    package_zip "$work_dir" "$DIST_DIR/drivesync-usb.zip"
    rm -rf "$work_dir"
}

build_auto() {
    local work_dir="$DIST_DIR/usb-build-auto"

    info "Building auto-clone edition..."
    create_base_structure "$work_dir"
    create_auto_config "$work_dir"
    create_auto_readme "$work_dir"
    create_syslinux_config "$work_dir"
    create_efi_stub "$work_dir"
    create_initramfs_placeholder "$work_dir"
    package_zip "$work_dir" "$DIST_DIR/drivesync-usb-auto.zip"
    rm -rf "$work_dir"
}

print_usage() {
    cat << EOF
DriveSync Bootable USB Builder

This script creates bootable USB images for DriveSync.

EDITIONS:
  Standard (drivesync-usb.zip):
    - Interactive mode with full TUI
    - Manual source/destination selection
    - Confirmation required before cloning

  Auto-Clone (drivesync-usb-auto.zip):
    - Automatic cloning, no interaction required
    - Clones internal drive to boot drive
    - Perfect for batch operations

USAGE:
  1. Run this script: ./boot/build-usb.sh
  2. Format a USB drive as FAT32 with GPT partition table
  3. Extract the appropriate zip to the USB drive root
  4. Boot from the USB drive

NOTE:
  For a fully bootable image, you need to add a Linux kernel and initramfs.
  See boot/BUILD-INSTRUCTIONS.txt in the extracted zip.

EOF
}

main() {
    info "DriveSync USB Image Builder"
    echo ""

    check_prerequisites
    build_standard
    build_auto

    echo ""
    info "Build complete!"
    info "Created: dist/drivesync-usb.zip (standard)"
    info "Created: dist/drivesync-usb-auto.zip (auto-clone)"
    echo ""
    print_usage
}

main "$@"
