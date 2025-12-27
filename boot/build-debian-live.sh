#!/bin/bash
#
# DriveSync Debian Live Bootable Image Builder
#
# Creates a full Debian Live environment (like Clonezilla) with DriveSync.
# Uses debootstrap for a proper Debian system with all drivers and tools.
#
# Requirements:
#   - Go binary already built (run 'make build' first)
#   - debootstrap, mksquashfs, xorriso
#   - Root privileges (for debootstrap)
#
# Output:
#   dist/drivesync-live-usb.zip - Full Debian Live USB image
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DIST_DIR="$PROJECT_DIR/dist"
BINARY="$DIST_DIR/drivesync-linux-amd64"

# Debian configuration
DEBIAN_RELEASE="bookworm"
DEBIAN_MIRROR="http://deb.debian.org/debian"
DEBIAN_SECURITY="http://security.debian.org/debian-security"

# Colors
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

    if [ "$EUID" -ne 0 ]; then
        error "This script must be run as root (for debootstrap)"
    fi

    local required_tools="debootstrap mksquashfs xorriso curl ar"
    for tool in $required_tools; do
        if ! command -v "$tool" &> /dev/null; then
            error "Required tool not found: $tool\nInstall with: apt install debootstrap squashfs-tools xorriso liblz4-tool"
        fi
    done

    info "Prerequisites OK"
}

download_signed_bootloader() {
    local cache_dir="$DIST_DIR/.debian-cache"
    local grub_dir="$cache_dir/signed-grub"

    if [ -d "$grub_dir" ] && [ -f "$grub_dir/shimx64.efi" ] && [ -f "$grub_dir/grubx64.efi" ]; then
        info "Using cached signed bootloader"
        echo "$grub_dir"
        return
    fi

    info "Downloading Debian signed bootloader..."
    mkdir -p "$grub_dir"
    local tmp_dir="$cache_dir/grub-tmp"
    mkdir -p "$tmp_dir"

    # Same versions as Clonezilla
    local shim_url="http://ftp.debian.org/debian/pool/main/s/shim-signed/shim-signed_1.47+15.8-1_amd64.deb"
    local grub_url="http://ftp.debian.org/debian/pool/main/g/grub-efi-amd64-signed/grub-efi-amd64-signed_1+2.12+9_amd64.deb"

    curl -sL -o "$tmp_dir/shim-signed.deb" "$shim_url" || error "Failed to download shim"
    curl -sL -o "$tmp_dir/grub-signed.deb" "$grub_url" || error "Failed to download grub"

    cd "$tmp_dir"
    ar x shim-signed.deb && tar -xf data.tar.* 2>/dev/null || true
    cp usr/lib/shim/shimx64.efi.signed "$grub_dir/shimx64.efi" 2>/dev/null || \
        find . -name "shimx64.efi*" -exec cp {} "$grub_dir/shimx64.efi" \; 2>/dev/null

    rm -f data.tar.* control.tar.* debian-binary
    ar x grub-signed.deb && tar -xf data.tar.* 2>/dev/null || true
    cp usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed "$grub_dir/grubx64.efi" 2>/dev/null || \
        find . -name "grubx64.efi*" -exec cp {} "$grub_dir/grubx64.efi" \; 2>/dev/null

    rm -rf "$tmp_dir"

    [ -f "$grub_dir/shimx64.efi" ] || error "Failed to extract shim"
    [ -f "$grub_dir/grubx64.efi" ] || error "Failed to extract grub"

    info "Signed bootloader ready"
    echo "$grub_dir"
}

create_debian_live() {
    local work_dir="$1"
    local cache_dir="$DIST_DIR/.debian-cache"
    local rootfs_dir="$cache_dir/debian-live-root"

    # Check if we can reuse cached rootfs
    if [ -d "$rootfs_dir" ] && [ -f "$rootfs_dir/.build-complete" ]; then
        info "Using cached Debian Live root filesystem"
    else
        info "Creating Debian Live root filesystem with debootstrap..."
        rm -rf "$rootfs_dir"
        mkdir -p "$rootfs_dir"

        # Debootstrap with essential packages
        info "Running debootstrap (this may take 5-10 minutes)..."
        debootstrap \
            --variant=minbase \
            --include=linux-image-amd64,live-boot,live-boot-initramfs-tools,\
initramfs-tools,systemd,systemd-sysv,udev,kmod,bash,\
pciutils,usbutils,util-linux,dosfstools,ntfs-3g,\
dbus,procps,iproute2,iputils-ping,ca-certificates \
            "$DEBIAN_RELEASE" \
            "$rootfs_dir" \
            "$DEBIAN_MIRROR"

        info "Configuring Debian Live system..."

        # Configure APT sources
        cat > "$rootfs_dir/etc/apt/sources.list" << EOF
deb $DEBIAN_MIRROR $DEBIAN_RELEASE main contrib non-free-firmware
deb $DEBIAN_SECURITY $DEBIAN_RELEASE-security main contrib non-free-firmware
deb $DEBIAN_MIRROR $DEBIAN_RELEASE-updates main contrib non-free-firmware
EOF

        # Install firmware packages via chroot (triggers initramfs rebuild)
        info "Installing firmware packages..."
        chroot "$rootfs_dir" /bin/bash -c "
            apt-get update
            apt-get install -y --no-install-recommends \
                firmware-linux-free \
                firmware-misc-nonfree \
                firmware-linux-nonfree
            apt-get clean
            rm -rf /var/lib/apt/lists/*
        "

        # Configure hostname
        echo "drivesync-live" > "$rootfs_dir/etc/hostname"

        # Configure network (DHCP on all interfaces)
        mkdir -p "$rootfs_dir/etc/network"
        cat > "$rootfs_dir/etc/network/interfaces" << 'EOF'
auto lo
iface lo inet loopback

allow-hotplug eth0
iface eth0 inet dhcp

allow-hotplug wlan0
iface wlan0 inet dhcp
EOF

        # Create fstab
        cat > "$rootfs_dir/etc/fstab" << 'EOF'
# DriveSync Live - minimal fstab
proc /proc proc defaults 0 0
sysfs /sys sysfs defaults 0 0
devpts /dev/pts devpts defaults 0 0
tmpfs /tmp tmpfs defaults 0 0
EOF

        # Configure systemd to auto-login and run DriveSync
        mkdir -p "$rootfs_dir/etc/systemd/system/getty@tty1.service.d"
        cat > "$rootfs_dir/etc/systemd/system/getty@tty1.service.d/autologin.conf" << 'EOF'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

        # Create DriveSync auto-start script
        cat > "$rootfs_dir/root/.profile" << 'EOF'
#!/bin/bash
# Auto-start DriveSync on login

if [ -z "$DISPLAY" ] && [ "$(tty)" = "/dev/tty1" ]; then
    clear
    exec /usr/bin/drivesync
fi
EOF

        # Mark build as complete
        touch "$rootfs_dir/.build-complete"
        info "Debian Live root filesystem created"
    fi

    # Copy DriveSync binary (always update this)
    info "Installing DriveSync binary..."
    cp "$BINARY" "$rootfs_dir/usr/bin/drivesync"
    chmod +x "$rootfs_dir/usr/bin/drivesync"

    # Create squashfs
    info "Creating squashfs (this may take a few minutes)..."
    mkdir -p "$work_dir/live"
    mksquashfs "$rootfs_dir" "$work_dir/live/filesystem.squashfs" \
        -comp xz -Xdict-size 100% -b 1M -no-recovery -quiet

    # Get kernel and initrd from the Debian system
    info "Extracting kernel and initrd..."
    local kernel=$(ls "$rootfs_dir/boot/vmlinuz-"* | head -1)
    local initrd=$(ls "$rootfs_dir/boot/initrd.img-"* | head -1)

    if [ -z "$kernel" ] || [ ! -f "$kernel" ]; then
        error "Kernel not found in Debian system"
    fi

    if [ -z "$initrd" ] || [ ! -f "$initrd" ]; then
        error "Initrd not found in Debian system"
    fi

    cp "$kernel" "$work_dir/live/vmlinuz"
    cp "$initrd" "$work_dir/live/initrd.img"

    local sqsize=$(stat -c%s "$work_dir/live/filesystem.squashfs" 2>/dev/null | numfmt --to=iec)
    info "Debian Live filesystem created: $sqsize"
}

create_boot_structure() {
    local work_dir="$1"
    local cache_dir="$DIST_DIR/.debian-cache"
    local grub_dir="$cache_dir/signed-grub"

    info "Creating boot structure..."

    rm -rf "$work_dir"
    mkdir -p "$work_dir/live" "$work_dir/EFI/BOOT" "$work_dir/boot/grub"

    # Create Debian Live filesystem
    create_debian_live "$work_dir"

    # Copy signed bootloader
    cp "$grub_dir/shimx64.efi" "$work_dir/EFI/BOOT/bootx64.efi"
    cp "$grub_dir/grubx64.efi" "$work_dir/EFI/BOOT/grubx64.efi"

    # GRUB configuration for Debian Live
    cat > "$work_dir/boot/grub/grub.cfg" << 'EOF'
set default="0"
set timeout="3"

insmod efi_gop
insmod efi_uga
insmod font
if loadfont ${prefix}/unicode.pf2; then
    insmod gfxterm
    set gfxmode=auto
    terminal_output gfxterm
fi

menuentry "DriveSync Live" {
    linux /live/vmlinuz boot=live components quiet splash
    initrd /live/initrd.img
}

menuentry "DriveSync Live (Debug - verbose boot)" {
    linux /live/vmlinuz boot=live components debug
    initrd /live/initrd.img
}

menuentry "DriveSync Live (Safe mode - no KMS)" {
    linux /live/vmlinuz boot=live components nomodeset
    initrd /live/initrd.img
}
EOF

    # Copy to other locations GRUB might search
    mkdir -p "$work_dir/EFI/debian"
    cp "$work_dir/boot/grub/grub.cfg" "$work_dir/EFI/debian/grub.cfg"
    cp "$work_dir/boot/grub/grub.cfg" "$work_dir/EFI/BOOT/grub.cfg"

    # Create README
    cat > "$work_dir/README.txt" << EOF
DriveSync Live USB - Debian Live Edition

Built on Debian $DEBIAN_RELEASE with full hardware support.
Uses signed bootloader for Secure Boot compatibility.

QUICK START:
1. Extract all files to a FAT32 USB drive
2. Boot from USB (UEFI mode)
3. Select "DriveSync Live" from menu
4. DriveSync will start automatically

BOOT OPTIONS:
- DriveSync Live: Normal boot (recommended)
- Debug mode: Verbose boot messages for troubleshooting
- Safe mode: Disable graphics acceleration (for compatibility)

FEATURES:
- Full Debian Linux with all drivers
- Automatic hardware detection (udev/systemd)
- Works on Surface Pro, Surface Book, and most modern PCs
- Secure Boot compatible

For more information: https://github.com/standardbeagle/drivesync
EOF
}

package_zip() {
    local work_dir="$1"
    local output_file="$2"

    info "Creating ZIP package: $output_file..."
    cd "$work_dir"
    zip -rq "$output_file" .

    local zipsize=$(stat -c%s "$output_file" 2>/dev/null | numfmt --to=iec)
    info "Created: $output_file ($zipsize)"
}

main() {
    info "DriveSync Debian Live USB Image Builder"
    info "Using full Debian $DEBIAN_RELEASE (like Clonezilla)"
    echo ""

    check_prerequisites

    mkdir -p "$DIST_DIR/.debian-cache"

    # Download signed bootloader
    download_signed_bootloader

    # Build the image
    local work_dir="$DIST_DIR/debian-live-build"
    local output_file="$DIST_DIR/drivesync-live-usb.zip"

    create_boot_structure "$work_dir"
    package_zip "$work_dir" "$output_file"

    # Clean up work dir but keep cache
    rm -rf "$work_dir"

    echo ""
    info "Build complete!"
    info "Created: dist/drivesync-live-usb.zip"
    echo ""
    echo "This image uses full Debian Live with:"
    echo "  - Debian $DEBIAN_RELEASE kernel and drivers"
    echo "  - Complete hardware support (udev/systemd)"
    echo "  - All diagnostic tools (lsblk, lspci, udevadm)"
    echo "  - Secure Boot compatible"
    echo ""
    echo "Tested on Surface Pro 7 and Surface Book."
    echo ""
}

main "$@"
