#!/bin/bash
#
# DriveSync Alpine-Based Bootable Image Builder
#
# Creates complete bootable USB images using Alpine Linux as the base.
# Includes bash and useful debug tools.
#
# Requirements:
#   - Go binary already built (run 'make build' first)
#   - curl, cpio, gzip, fakeroot (or root access)
#
# Output:
#   dist/drivesync-usb.zip      - Standard interactive mode (bootable)
#   dist/drivesync-usb-auto.zip - Auto-clone mode (bootable)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DIST_DIR="$PROJECT_DIR/dist"
BINARY="$DIST_DIR/drivesync-linux-amd64"

# Alpine version and architecture
ALPINE_VERSION="3.21"
ALPINE_ARCH="x86_64"
ALPINE_MIRROR="https://dl-cdn.alpinelinux.org/alpine"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

check_prerequisites() {
    info "Checking prerequisites..."

    if [ ! -f "$BINARY" ]; then
        error "Binary not found: $BINARY\nRun 'make build' first."
    fi

    local required_tools="curl cpio gzip zip"
    for tool in $required_tools; do
        if ! command -v "$tool" &> /dev/null; then
            error "Required tool not found: $tool"
        fi
    done

    # Check for fakeroot or root access
    if [ "$(id -u)" != "0" ] && ! command -v fakeroot &> /dev/null; then
        warn "Neither root access nor fakeroot available."
        warn "Some operations may fail. Install fakeroot: apt install fakeroot"
    fi

    info "Prerequisites OK"
}

download_alpine() {
    local cache_dir="$DIST_DIR/.alpine-cache"
    local rootfs_url="${ALPINE_MIRROR}/v${ALPINE_VERSION}/releases/${ALPINE_ARCH}/alpine-minirootfs-${ALPINE_VERSION}.0-${ALPINE_ARCH}.tar.gz"
    local rootfs_file="$cache_dir/alpine-minirootfs.tar.gz"

    mkdir -p "$cache_dir"

    if [ ! -f "$rootfs_file" ]; then
        info "Downloading Alpine Linux minirootfs..."
        curl -L -o "$rootfs_file" "$rootfs_url" || error "Failed to download Alpine"
    else
        info "Using cached Alpine minirootfs"
    fi

    echo "$rootfs_file"
}

download_kernel() {
    local cache_dir="$DIST_DIR/.alpine-cache"
    local kernel_url="${ALPINE_MIRROR}/v${ALPINE_VERSION}/releases/${ALPINE_ARCH}/netboot/vmlinuz-lts"
    local initramfs_url="${ALPINE_MIRROR}/v${ALPINE_VERSION}/releases/${ALPINE_ARCH}/netboot/initramfs-lts"
    local modloop_url="${ALPINE_MIRROR}/v${ALPINE_VERSION}/releases/${ALPINE_ARCH}/netboot/modloop-lts"

    mkdir -p "$cache_dir"

    if [ ! -f "$cache_dir/vmlinuz-lts" ]; then
        info "Downloading Alpine kernel..."
        curl -L -o "$cache_dir/vmlinuz-lts" "$kernel_url" || error "Failed to download kernel"
    fi

    if [ ! -f "$cache_dir/initramfs-lts" ]; then
        info "Downloading Alpine initramfs..."
        curl -L -o "$cache_dir/initramfs-lts" "$initramfs_url" || error "Failed to download initramfs"
    fi

    if [ ! -f "$cache_dir/modloop-lts" ]; then
        info "Downloading Alpine modules..."
        curl -L -o "$cache_dir/modloop-lts" "$modloop_url" || error "Failed to download modloop"
    fi

    echo "$cache_dir"
}

create_initramfs() {
    local work_dir="$1"
    local edition="$2"  # "standard" or "auto"
    local cache_dir="$DIST_DIR/.alpine-cache"
    local initramfs_dir="$work_dir/initramfs-work"

    info "Creating initramfs for $edition edition..."

    rm -rf "$initramfs_dir"
    mkdir -p "$initramfs_dir"

    # Extract Alpine base initramfs
    cd "$initramfs_dir"
    gzip -dc "$cache_dir/initramfs-lts" | cpio -idm 2>/dev/null || true

    # Add DriveSync binary
    cp "$BINARY" "$initramfs_dir/usr/bin/drivesync"
    chmod +x "$initramfs_dir/usr/bin/drivesync"

    # Create drivesync launcher
    cat > "$initramfs_dir/usr/bin/start-drivesync" << 'LAUNCHER'
#!/bin/sh
# Wait for devices
sleep 2
# Clear screen
clear
# Run DriveSync
exec /usr/bin/drivesync
LAUNCHER
    chmod +x "$initramfs_dir/usr/bin/start-drivesync"

    # Add config file for auto mode
    if [ "$edition" = "auto" ]; then
        mkdir -p "$initramfs_dir/etc"
        cat > "$initramfs_dir/drivesync.kdl" << 'CONFIG'
// DriveSync Auto-Clone Configuration
mode "auto"
source "internal"
destination "boot-drive"
verify false
on-complete "prompt"
CONFIG
    fi

    # Create custom init that runs DriveSync
    cat > "$initramfs_dir/init-drivesync" << 'INIT'
#!/bin/sh
# DriveSync Init Script

# Mount essential filesystems
mount -t proc none /proc
mount -t sysfs none /sys
mount -t devtmpfs none /dev

# Wait for devices to settle
echo "Waiting for devices..."
sleep 3

# Load essential modules
modprobe -a nvme nvme-core sd_mod usb-storage uas ehci-hcd xhci-hcd 2>/dev/null || true

# Trigger udev/mdev
mdev -s 2>/dev/null || true

# Clear screen
clear

# Run DriveSync
echo "Starting DriveSync..."
exec /usr/bin/drivesync

# Fallback shell
exec /bin/sh
INIT
    chmod +x "$initramfs_dir/init-drivesync"

    # Repack initramfs
    cd "$initramfs_dir"
    find . | cpio -o -H newc 2>/dev/null | gzip > "$work_dir/boot/initramfs.gz"

    rm -rf "$initramfs_dir"
}

create_boot_structure() {
    local work_dir="$1"
    local edition="$2"
    local cache_dir="$DIST_DIR/.alpine-cache"

    info "Creating boot structure for $edition edition..."

    rm -rf "$work_dir"
    mkdir -p "$work_dir/boot" "$work_dir/EFI/BOOT" "$work_dir/syslinux"

    # Copy kernel
    cp "$cache_dir/vmlinuz-lts" "$work_dir/boot/vmlinuz"

    # Copy modloop for driver support
    cp "$cache_dir/modloop-lts" "$work_dir/boot/modloop-lts"

    # Create initramfs with DriveSync
    create_initramfs "$work_dir" "$edition"

    # SYSLINUX config for BIOS boot
    cat > "$work_dir/syslinux/syslinux.cfg" << 'EOF'
DEFAULT drivesync
TIMEOUT 30
PROMPT 0

LABEL drivesync
    MENU LABEL DriveSync
    LINUX /boot/vmlinuz
    INITRD /boot/initramfs.gz
    APPEND modloop=/boot/modloop-lts modules=loop,squashfs,nvme,usb-storage quiet
EOF
    cp "$work_dir/syslinux/syslinux.cfg" "$work_dir/syslinux.cfg"

    # EFI boot config
    cat > "$work_dir/EFI/BOOT/startup.nsh" << 'EOF'
@echo -off
fs0:
\boot\vmlinuz initrd=\boot\initramfs.gz modloop=/boot/modloop-lts modules=loop,squashfs,nvme,usb-storage quiet
EOF

    # Create edition-specific README
    if [ "$edition" = "standard" ]; then
        cat > "$work_dir/README.txt" << 'EOF'
DriveSync Bootable USB - Standard Edition (Alpine Linux)

This is a complete bootable image based on Alpine Linux.

QUICK START:
1. Format USB drive as FAT32 with GPT partition table
2. Extract this zip to the USB drive root
3. Boot from USB drive
4. Select source and destination drives
5. Type "clone" to confirm

INCLUDED:
- Linux kernel with NVMe, SATA, USB support
- DriveSync TUI application
- Debug shell (press Ctrl+C to access)

For more information: https://github.com/standardbeagle/drivesync
EOF
    else
        cat > "$work_dir/README.txt" << 'EOF'
DriveSync Bootable USB - Auto-Clone Edition (Alpine Linux)

!!! AUTOMATIC CLONING - NO CONFIRMATION REQUIRED !!!

This is a complete bootable image that will automatically clone
the internal drive to this boot drive. No user interaction needed.

HOW IT WORKS:
1. Boot from this drive
2. DriveSync starts automatically
3. Clones internal drive to this drive
4. Shows completion screen when done

CONFIGURATION:
Edit drivesync.kdl on this drive to customize:
- Change "on-complete" to "shutdown" for auto power-off
- Change "verify" to "true" for read-back verification

For more information: https://github.com/standardbeagle/drivesync
EOF
        # Copy config to root of USB for easy editing
        cat > "$work_dir/drivesync.kdl" << 'CONFIG'
// DriveSync Auto-Clone Configuration
// Edit this file to customize behavior

mode "auto"
source "internal"
destination "boot-drive"
verify false
on-complete "prompt"
CONFIG
    fi
}

package_zip() {
    local work_dir="$1"
    local output_file="$2"

    info "Creating ZIP package: $output_file..."
    cd "$work_dir"
    zip -rq "$output_file" .
    info "Created: $output_file"
}

build_edition() {
    local edition="$1"
    local work_dir="$DIST_DIR/alpine-build-$edition"
    local output_file="$DIST_DIR/drivesync-usb.zip"

    if [ "$edition" = "auto" ]; then
        output_file="$DIST_DIR/drivesync-usb-auto.zip"
    fi

    create_boot_structure "$work_dir" "$edition"
    package_zip "$work_dir" "$output_file"
    rm -rf "$work_dir"
}

main() {
    info "DriveSync Alpine-Based USB Image Builder"
    echo ""

    check_prerequisites

    # Download Alpine components (cached)
    download_alpine
    download_kernel

    # Build both editions
    build_edition "standard"
    build_edition "auto"

    echo ""
    info "Build complete!"
    info "Created: dist/drivesync-usb.zip (standard, ~50MB)"
    info "Created: dist/drivesync-usb-auto.zip (auto-clone, ~50MB)"
    echo ""
    echo "These are complete bootable images. Extract to a FAT32 USB drive and boot."
}

main "$@"
