VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
BINARY := drivesync-linux-amd64

.PHONY: all build clean test test-integration usb docs release

all: build

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY) ./cmd/drivesync

build-local:
	go build $(LDFLAGS) -o dist/drivesync ./cmd/drivesync

clean:
	rm -rf dist/

test:
	go test -v ./...

test-integration:
	sudo go test -tags=integration -v ./...

test-race:
	go test -race -v ./...

lint:
	golangci-lint run

fmt:
	go fmt ./...
	goimports -w .

vet:
	go vet ./...

# Create bootable USB image (placeholder structure only)
usb: build
	./boot/build-usb.sh

# Create complete bootable USB image with Debian Live
usb-live: build
	./boot/build-debian-live.sh

# Create release artifacts
release: build usb
	@echo "Creating release $(VERSION)"
	cd dist && tar -czf drivesync-$(VERSION).tar.gz $(BINARY)
	@echo "Artifacts in dist/"

# Install locally for testing
install: build-local
	sudo cp dist/drivesync /usr/local/bin/

# Run the application (requires root)
run: build-local
	sudo ./dist/drivesync

# Generate test fixtures
fixtures:
	./scripts/create-test-fixtures.sh testdata/

# Check if build tools are available
check-tools:
	@which go > /dev/null || (echo "go not found" && exit 1)
	@which git > /dev/null || (echo "git not found" && exit 1)
	@echo "All tools available"

# Show help
help:
	@echo "DriveSync Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build Linux AMD64 binary"
	@echo "  build-local    Build for current platform"
	@echo "  clean          Remove build artifacts"
	@echo "  test           Run unit tests"
	@echo "  test-integration  Run integration tests (requires sudo)"
	@echo "  test-race      Run tests with race detector"
	@echo "  lint           Run linter"
	@echo "  fmt            Format code"
	@echo "  vet            Run go vet"
	@echo "  usb            Create bootable USB image (placeholder)"
	@echo "  usb-live       Create complete bootable USB with Debian Live"
	@echo "  release        Create release artifacts"
	@echo "  install        Install binary to /usr/local/bin"
	@echo "  run            Build and run (requires sudo)"
	@echo "  fixtures       Generate test fixtures"
	@echo "  check-tools    Verify build tools are available"
