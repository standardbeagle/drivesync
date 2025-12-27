# Building DriveSync

This guide covers building DriveSync from source for all platforms.

## Prerequisites

### All Platforms
- Go 1.22 or later
- Git

### For Bootable USB Images (Linux only)
- Debian or Ubuntu host (for debootstrap)
- Root privileges (sudo)
- Build tools:
  ```bash
  sudo apt-get install debootstrap squashfs-tools xorriso liblz4-tool zip
  ```

## Quick Build

### Windows Executable
```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=$(git describe --tags --always)" \
  -o dist/drivesync.exe ./cmd/drivesync
```

Creates: `dist/drivesync.exe`

### Linux Static Binary
```bash
make build
```

Or manually:
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=$(git describe --tags --always)" \
  -o dist/drivesync-linux-amd64 ./cmd/drivesync
```

Creates: `dist/drivesync-linux-amd64`

### Bootable USB Image (Debian Live)
```bash
sudo make usb-live
```

Creates: `dist/drivesync-live-usb.zip`

**Note:** Requires Linux host with debootstrap. Takes 5-15 minutes depending on network speed.

## Makefile Targets

```bash
make build          # Build Linux static binary
make build-local    # Build for current platform (development)
make test           # Run unit tests
make test-race      # Run tests with race detector
make test-integration  # Run integration tests (requires sudo)
make lint           # Run golangci-lint
make fmt            # Format code
make usb-live       # Create Debian Live bootable USB
make clean          # Remove build artifacts
make install        # Install to /usr/local/bin (Linux only)
```

## Development Workflow

### 1. Clone Repository
```bash
git clone https://github.com/standardbeagle/drivesync.git
cd drivesync
```

### 2. Install Dependencies
```bash
go mod download
```

### 3. Run Tests
```bash
make test
make test-race
```

### 4. Build for Testing
```bash
make build-local    # Builds for your current OS/arch
```

### 5. Run Locally
```bash
# Linux
sudo ./dist/drivesync

# Windows (PowerShell as Administrator)
.\dist\drivesync.exe
```

## Cross-Compilation

DriveSync is pure Go with no CGO dependencies, enabling easy cross-compilation:

```bash
# Windows from Linux/Mac
GOOS=windows GOARCH=amd64 go build -o dist/drivesync.exe ./cmd/drivesync

# Linux from Windows/Mac
GOOS=linux GOARCH=amd64 go build -o dist/drivesync-linux ./cmd/drivesync

# ARM64 (e.g., Raspberry Pi, Apple Silicon)
GOOS=linux GOARCH=arm64 go build -o dist/drivesync-arm64 ./cmd/drivesync
```

## CI/CD

GitHub Actions automatically:
- Run tests on every push/PR
- Build Linux binary for verification
- Create releases when tags are pushed

### Creating a Release

1. Tag a version:
   ```bash
   git tag v0.3.0
   git push origin v0.3.0
   ```

2. GitHub Actions automatically:
   - Runs full test suite
   - Builds Windows executable (zip)
   - Builds Linux static binary
   - Builds Debian Live USB image (zip)
   - Creates GitHub release with all artifacts

## Bootable USB Details

The `make usb-live` target creates a complete Debian Live USB with:
- Minimal Debian base (no GUI, networking, or unnecessary services)
- Linux kernel with storage drivers
- DriveSync binary auto-starts on boot
- Secure Boot compatible (signed GRUB bootloader)
- UEFI and legacy BIOS support

### Customizing the USB Image

Edit `boot/build-debian-live.sh` to:
- Change Debian release (default: bookworm)
- Add custom packages
- Modify boot parameters
- Include additional firmware

### Testing USB Image Locally

```bash
# Extract the zip
unzip dist/drivesync-live-usb.zip -d /tmp/usb-test

# Test with QEMU (if installed)
qemu-system-x86_64 -enable-kvm -m 2048 \
  -drive file=/tmp/usb-test/EFI/BOOT/bootx64.efi,format=raw \
  -bios /usr/share/ovmf/OVMF.fd
```

## Troubleshooting

### "go: go.mod file not found"
```bash
# Make sure you're in the project root
cd drivesync
```

### "permission denied" when building USB
```bash
# USB build requires root for debootstrap
sudo make usb-live
```

### Tests fail with "permission denied"
```bash
# Some tests require root to access block devices
sudo make test-integration
```

### Windows build missing go-ole dependency
```bash
# Ensure dependencies are downloaded
go mod download
```

## Performance Optimization

### Build Flags

The `-ldflags="-s -w"` flags:
- `-s`: Strip symbol table (reduces binary size ~30%)
- `-w`: Strip DWARF debug info (reduces binary size ~10%)

For debugging, omit these flags:
```bash
go build -o dist/drivesync-debug ./cmd/drivesync
```

### Static Linking

`CGO_ENABLED=0` ensures fully static binaries:
- No external dependencies
- Works on any Linux (even musl-based like Alpine)
- Single executable deployment

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for:
- Code style guidelines
- Testing requirements
- Pull request process
