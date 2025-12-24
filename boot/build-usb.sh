#!/bin/bash
#
# DriveSync Bootable USB Image Builder
#
# This script creates a bootable USB image containing DriveSync.
# The image uses a minimal Linux environment that boots directly into DriveSync.
#
# Requirements:
#   - Go binary already built (run 'make build' first)
#   - mksquashfs, genisoimage (or xorriso), syslinux
#   - Root access for some operations
#
# Output:
#   dist/drivesync-usb.zip - Ready to extract to a FAT32 formatted USB drive
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

create_minimal_bootable() {
    info "Creating minimal bootable structure..."

    # Clean up any previous build
    rm -rf "$WORK_DIR"
    mkdir -p "$WORK_DIR"

    # Create EFI boot structure
    mkdir -p "$WORK_DIR/EFI/BOOT"
    mkdir -p "$WORK_DIR/boot"

    # Copy DriveSync binary
    cp "$BINARY" "$WORK_DIR/drivesync"
    chmod +x "$WORK_DIR/drivesync"

    # Create a startup script
    cat > "$WORK_DIR/start-drivesync.sh" << 'EOF'
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
    chmod +x "$WORK_DIR/start-drivesync.sh"

    # Create README for the USB drive
    cat > "$WORK_DIR/README.txt" << 'EOF'
DriveSync Bootable USB

QUICK START:
1. Restart your computer
2. Enter BIOS/UEFI boot menu (usually F12, F2, or Del at startup)
3. Select this USB drive
4. DriveSync will start automatically

SELF-OVERWRITE MODE:
If you extracted DriveSync to the destination drive instead of a USB stick,
DriveSync will detect this and offer one-click cloning of your internal drive.

For more information: https://github.com/beagle/drivesync
EOF

    info "Bootable structure created"
}

create_syslinux_config() {
    info "Creating SYSLINUX configuration..."

    mkdir -p "$WORK_DIR/syslinux"

    # SYSLINUX config for BIOS boot
    cat > "$WORK_DIR/syslinux/syslinux.cfg" << 'EOF'
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
    cp "$WORK_DIR/syslinux/syslinux.cfg" "$WORK_DIR/syslinux.cfg"

    info "SYSLINUX configuration created"
}

create_efi_stub() {
    info "Creating EFI boot stub..."

    # Create a simple EFI startup script
    # In production, this would be a proper EFI application or GRUB
    cat > "$WORK_DIR/EFI/BOOT/startup.nsh" << 'EOF'
@echo -off
echo DriveSync Boot
fs0:
cd \boot
vmlinuz initrd=\boot\initramfs.cpio.gz toram quiet
EOF

    info "EFI boot stub created"
}

create_initramfs_placeholder() {
    info "Creating initramfs placeholder..."

    # Create placeholder instructions
    cat > "$WORK_DIR/boot/BUILD-INSTRUCTIONS.txt" << 'EOF'
BUILDING A COMPLETE BOOTABLE IMAGE
===================================

To create a fully bootable DriveSync USB, you need:

1. A Linux kernel (vmlinuz)
2. An initramfs containing:
   - BusyBox or toybox for basic utilities
   - Device drivers (NVMe, USB, SATA)
   - The DriveSync binary
   - An init script that runs DriveSync

RECOMMENDED BASE SYSTEMS:
- Alpine Linux (minimal, musl-based)
- Void Linux (minimal, good driver support)
- Debian Live (stable, comprehensive drivers)

QUICK METHOD WITH ALPINE:
1. Download Alpine "extended" ISO
2. Extract boot/vmlinuz-lts and boot/initramfs-lts
3. Repack initramfs with DriveSync added

CUSTOM INITRAMFS:
See scripts/build-initramfs.sh for a complete example.

The resulting files should be:
- boot/vmlinuz      (Linux kernel)
- boot/initramfs.cpio.gz  (Initial RAM filesystem)
EOF

    info "Initramfs placeholder created"
}

package_zip() {
    info "Creating ZIP package..."

    cd "$WORK_DIR"
    zip -r "$DIST_DIR/drivesync-usb.zip" .

    info "Created: $DIST_DIR/drivesync-usb.zip"
}

print_usage() {
    cat << EOF
DriveSync Bootable USB Builder

This script creates the bootable USB structure for DriveSync.

USAGE:
  To create a bootable USB drive:

  1. Run this script: ./boot/build-usb.sh
  2. Format a USB drive as FAT32 with GPT partition table
  3. Extract drivesync-usb.zip to the USB drive root
  4. Boot from the USB drive

NOTE:
  For a fully bootable image, you need to add a Linux kernel and initramfs.
  See dist/usb-build/boot/BUILD-INSTRUCTIONS.txt after running this script.

FILES CREATED:
  dist/drivesync-usb.zip    - Extract to USB drive
  dist/usb-build/           - Working directory (can be deleted)

EOF
}

main() {
    info "DriveSync USB Image Builder"
    echo ""

    check_prerequisites
    create_minimal_bootable
    create_syslinux_config
    create_efi_stub
    create_initramfs_placeholder
    package_zip

    echo ""
    info "Build complete!"
    echo ""
    print_usage
}

main "$@"
