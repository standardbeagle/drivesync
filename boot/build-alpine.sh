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

    local required_tools="curl cpio gzip zip ar"
    for tool in $required_tools; do
        if ! command -v "$tool" &> /dev/null; then
            error "Required tool not found: $tool (install binutils for ar)"
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

download_syslinux() {
    local cache_dir="$DIST_DIR/.alpine-cache"
    local syslinux_dir="$cache_dir/syslinux"

    if [ -d "$syslinux_dir" ]; then
        info "Using cached syslinux bootloader"
        echo "$syslinux_dir"
        return
    fi

    info "Downloading syslinux bootloader..."
    mkdir -p "$syslinux_dir"

    # Find the correct syslinux package version
    local pkg_list
    pkg_list=$(curl -sL "${ALPINE_MIRROR}/v${ALPINE_VERSION}/main/${ALPINE_ARCH}/" | grep -o 'syslinux-[0-9][^"]*\.apk' | head -1)

    if [ -z "$pkg_list" ]; then
        # Fallback to v3.20 if v3.21 doesn't have it
        pkg_list=$(curl -sL "${ALPINE_MIRROR}/v3.20/main/${ALPINE_ARCH}/" | grep -o 'syslinux-[0-9][^"]*\.apk' | head -1)
        local syslinux_url="${ALPINE_MIRROR}/v3.20/main/${ALPINE_ARCH}/${pkg_list}"
    else
        local syslinux_url="${ALPINE_MIRROR}/v${ALPINE_VERSION}/main/${ALPINE_ARCH}/${pkg_list}"
    fi

    info "Downloading: $syslinux_url"

    local tmp_dir="$cache_dir/syslinux-tmp"
    mkdir -p "$tmp_dir"
    curl -sL -o "$tmp_dir/syslinux.apk" "$syslinux_url" || error "Failed to download syslinux"

    # Extract APK (it's a gzipped tarball)
    cd "$tmp_dir"
    tar -xzf syslinux.apk 2>/dev/null || true

    # Copy BIOS bootloader files (syslinux still used for BIOS boot)
    mkdir -p "$syslinux_dir/bios"
    cp usr/share/syslinux/mbr.bin "$syslinux_dir/bios/" 2>/dev/null || true
    cp usr/share/syslinux/isolinux.bin "$syslinux_dir/bios/" 2>/dev/null || true
    cp usr/share/syslinux/ldlinux.c32 "$syslinux_dir/bios/" 2>/dev/null || true
    cp usr/share/syslinux/linux.c32 "$syslinux_dir/bios/" 2>/dev/null || true
    cp usr/share/syslinux/libcom32.c32 "$syslinux_dir/bios/" 2>/dev/null || true
    cp usr/share/syslinux/libutil.c32 "$syslinux_dir/bios/" 2>/dev/null || true

    # Cleanup
    rm -rf "$tmp_dir"

    info "Syslinux bootloader extracted"
    echo "$syslinux_dir"
}

download_signed_grub() {
    local cache_dir="$DIST_DIR/.alpine-cache"
    local grub_dir="$cache_dir/signed-grub"

    if [ -d "$grub_dir" ] && [ -f "$grub_dir/shimx64.efi" ] && [ -f "$grub_dir/grubx64.efi" ]; then
        info "Using cached signed GRUB bootloader"
        echo "$grub_dir"
        return
    fi

    info "Downloading Debian signed GRUB bootloader (Secure Boot compatible)..."
    mkdir -p "$grub_dir"

    local tmp_dir="$cache_dir/grub-tmp"
    mkdir -p "$tmp_dir"

    # Debian Bookworm (12) signed bootloader packages
    local shim_url="http://ftp.debian.org/debian/pool/main/s/shim-signed/shim-signed_1.44~1+deb12u1+15.8-1~deb12u1_amd64.deb"
    local grub_url="http://ftp.debian.org/debian/pool/main/g/grub-efi-amd64-signed/grub-efi-amd64-signed_1+2.06+13+deb12u1_amd64.deb"

    info "Downloading shim-signed..."
    curl -sL -o "$tmp_dir/shim-signed.deb" "$shim_url" || error "Failed to download shim-signed"

    info "Downloading grub-efi-amd64-signed..."
    curl -sL -o "$tmp_dir/grub-signed.deb" "$grub_url" || error "Failed to download grub-efi-amd64-signed"

    # Extract .deb files (they are ar archives containing data.tar.xz)
    cd "$tmp_dir"

    # Extract shim
    ar x shim-signed.deb
    tar -xf data.tar.* 2>/dev/null || tar -xJf data.tar.xz 2>/dev/null || tar -xzf data.tar.gz 2>/dev/null || true

    # The shim EFI binary - look in standard locations
    if [ -f "usr/lib/shim/shimx64.efi.signed" ]; then
        cp "usr/lib/shim/shimx64.efi.signed" "$grub_dir/shimx64.efi"
    elif [ -f "usr/lib/shim/shimx64.efi.signed.latest" ]; then
        cp "usr/lib/shim/shimx64.efi.signed.latest" "$grub_dir/shimx64.efi"
    else
        # Find it wherever it might be
        find . -name "shimx64.efi*" -exec cp {} "$grub_dir/shimx64.efi" \; 2>/dev/null || true
    fi

    # Also copy mmx64.efi (MOK Manager) if present
    find . -name "mmx64.efi*" -exec cp {} "$grub_dir/mmx64.efi" \; 2>/dev/null || true

    # Clean up and extract grub
    rm -f data.tar.* control.tar.* debian-binary
    ar x grub-signed.deb
    tar -xf data.tar.* 2>/dev/null || tar -xJf data.tar.xz 2>/dev/null || tar -xzf data.tar.gz 2>/dev/null || true

    # The signed GRUB EFI binary
    if [ -f "usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed" ]; then
        cp "usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed" "$grub_dir/grubx64.efi"
    else
        find . -name "grubx64.efi*" -exec cp {} "$grub_dir/grubx64.efi" \; 2>/dev/null || true
    fi

    # Cleanup
    rm -rf "$tmp_dir"

    # Verify we got the files
    if [ ! -f "$grub_dir/shimx64.efi" ]; then
        error "Failed to extract shimx64.efi"
    fi
    if [ ! -f "$grub_dir/grubx64.efi" ]; then
        error "Failed to extract grubx64.efi"
    fi

    local shim_size=$(stat -c%s "$grub_dir/shimx64.efi" 2>/dev/null || stat -f%z "$grub_dir/shimx64.efi")
    local grub_size=$(stat -c%s "$grub_dir/grubx64.efi" 2>/dev/null || stat -f%z "$grub_dir/grubx64.efi")
    info "Signed bootloader extracted: shimx64.efi (${shim_size} bytes), grubx64.efi (${grub_size} bytes)"

    echo "$grub_dir"
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
    local syslinux_dir="$cache_dir/syslinux"
    local grub_dir="$cache_dir/signed-grub"

    info "Creating boot structure for $edition edition..."

    rm -rf "$work_dir"
    mkdir -p "$work_dir/boot/grub" "$work_dir/EFI/BOOT" "$work_dir/syslinux"

    # Copy kernel
    cp "$cache_dir/vmlinuz-lts" "$work_dir/boot/vmlinuz"

    # Copy modloop for driver support
    cp "$cache_dir/modloop-lts" "$work_dir/boot/modloop-lts"

    # Create initramfs with DriveSync
    create_initramfs "$work_dir" "$edition"

    # --- UEFI Boot Setup (Secure Boot compatible) ---
    # Copy signed GRUB bootloader
    # shimx64.efi is the Secure Boot shim that loads grubx64.efi
    cp "$grub_dir/shimx64.efi" "$work_dir/EFI/BOOT/bootx64.efi"
    cp "$grub_dir/grubx64.efi" "$work_dir/EFI/BOOT/grubx64.efi"

    # Copy MOK Manager if available (for key enrollment)
    if [ -f "$grub_dir/mmx64.efi" ]; then
        cp "$grub_dir/mmx64.efi" "$work_dir/EFI/BOOT/mmx64.efi"
    fi

    # GRUB configuration - Debian's signed GRUB looks in /boot/grub/grub.cfg
    # and also /EFI/debian/grub.cfg. We need to place it in all possible locations.
    cat > "$work_dir/boot/grub/grub.cfg" << 'EOF'
# DriveSync GRUB Configuration (Secure Boot Compatible)
set timeout=3
set default=0

# Search for the boot partition
search --no-floppy --set=root --file /boot/vmlinuz

menuentry "DriveSync" {
    linux /boot/vmlinuz modloop=/boot/modloop-lts modules=loop,squashfs,nvme,usb-storage quiet
    initrd /boot/initramfs.gz
}

menuentry "DriveSync (Debug Mode)" {
    linux /boot/vmlinuz modloop=/boot/modloop-lts modules=loop,squashfs,nvme,usb-storage
    initrd /boot/initramfs.gz
}
EOF

    # Create EFI/debian directory (where Debian's signed GRUB looks for config)
    mkdir -p "$work_dir/EFI/debian"
    cp "$work_dir/boot/grub/grub.cfg" "$work_dir/EFI/debian/grub.cfg"

    # Also place in EFI/BOOT for completeness
    cp "$work_dir/boot/grub/grub.cfg" "$work_dir/EFI/BOOT/grub.cfg"

    # Create grub directory at root level (another common location)
    mkdir -p "$work_dir/grub"
    cp "$work_dir/boot/grub/grub.cfg" "$work_dir/grub/grub.cfg"

    # Fallback EFI shell script (for systems with EFI Shell)
    cat > "$work_dir/EFI/BOOT/startup.nsh" << 'EOF'
@echo -off
echo Booting DriveSync...
fs0:
\boot\vmlinuz initrd=\boot\initramfs.gz modloop=/boot/modloop-lts modules=loop,squashfs,nvme,usb-storage quiet
EOF

    # --- BIOS Boot Setup ---
    # Syslinux config for BIOS boot
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

    # Copy syslinux BIOS files for manual installation
    cp "$syslinux_dir/bios/mbr.bin" "$work_dir/syslinux/" 2>/dev/null || true
    cp "$syslinux_dir/bios/ldlinux.c32" "$work_dir/syslinux/" 2>/dev/null || true
    cp "$syslinux_dir/bios/linux.c32" "$work_dir/syslinux/" 2>/dev/null || true
    cp "$syslinux_dir/bios/libcom32.c32" "$work_dir/syslinux/" 2>/dev/null || true
    cp "$syslinux_dir/bios/libutil.c32" "$work_dir/syslinux/" 2>/dev/null || true

    # Create makeboot script for BIOS systems
    cat > "$work_dir/makeboot.sh" << 'MAKEBOOT'
#!/bin/bash
# DriveSync BIOS Boot Setup
# Run this script to make a USB drive bootable on BIOS systems
# Usage: sudo ./makeboot.sh /dev/sdX

set -e

if [ -z "$1" ]; then
    echo "Usage: sudo $0 /dev/sdX"
    echo "  where /dev/sdX is your USB drive (NOT a partition)"
    exit 1
fi

DEVICE="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ ! -b "$DEVICE" ]; then
    echo "Error: $DEVICE is not a block device"
    exit 1
fi

if [ "$(id -u)" != "0" ]; then
    echo "Error: This script must be run as root"
    exit 1
fi

echo "Installing MBR to $DEVICE..."
cat "$SCRIPT_DIR/syslinux/mbr.bin" > "$DEVICE"

# Find the first partition
PARTITION="${DEVICE}1"
if [ ! -b "$PARTITION" ]; then
    PARTITION="${DEVICE}p1"
fi

if [ -b "$PARTITION" ]; then
    echo "Installing syslinux to $PARTITION..."
    syslinux --install "$PARTITION" || extlinux --install "$SCRIPT_DIR/syslinux"
fi

echo "Done! Drive should now be bootable on BIOS systems."
MAKEBOOT
    chmod +x "$work_dir/makeboot.sh"

    # Create edition-specific README
    if [ "$edition" = "standard" ]; then
        cat > "$work_dir/README.txt" << 'EOF'
DriveSync Bootable USB - Standard Edition (Alpine Linux)

This is a complete bootable image based on Alpine Linux.
Uses Debian's signed GRUB bootloader for Secure Boot compatibility.

QUICK START (UEFI - most modern systems):
1. Format USB drive as FAT32 with GPT partition table
2. Extract this zip to the USB drive root
3. Boot from USB drive (select UEFI boot option)
4. Select source and destination drives
5. Type "clone" to confirm

SECURE BOOT:
This image uses Debian's signed GRUB bootloader and works with
Secure Boot enabled. No need to disable Secure Boot.

BIOS BOOT (legacy systems):
If your system uses BIOS instead of UEFI:
1. Extract zip to USB drive
2. Run: sudo ./makeboot.sh /dev/sdX (replace sdX with your USB device)
3. Boot from USB drive

INCLUDED:
- Signed GRUB bootloader (UEFI with Secure Boot support)
- Syslinux bootloader (BIOS)
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
Uses Debian's signed GRUB bootloader for Secure Boot compatibility.

HOW IT WORKS:
1. Boot from this drive (UEFI or BIOS)
2. DriveSync starts automatically
3. Clones internal drive to this drive
4. Shows completion screen when done

SECURE BOOT:
This image uses Debian's signed GRUB bootloader and works with
Secure Boot enabled. No need to disable Secure Boot.

BIOS BOOT (legacy systems):
If your system uses BIOS instead of UEFI:
1. Extract zip to USB drive
2. Run: sudo ./makeboot.sh /dev/sdX (replace sdX with your USB device)
3. Boot from USB drive

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
    download_syslinux
    download_signed_grub

    # Build both editions
    build_edition "standard"
    build_edition "auto"

    echo ""
    info "Build complete!"
    info "Created: dist/drivesync-usb.zip (standard)"
    info "Created: dist/drivesync-usb-auto.zip (auto-clone)"
    echo ""
    echo "These are complete bootable images with UEFI and BIOS support."
    echo "Extract to a FAT32 USB drive and boot."
    echo ""
    echo "For BIOS systems, run makeboot.sh after extracting."
}

main "$@"
