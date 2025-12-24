// Package config handles DriveSync configuration file parsing.
// Configuration files use KDL format and can pre-configure clone operations.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sblinch/kdl-go"
	"github.com/sblinch/kdl-go/document"
)

// Mode determines how DriveSync operates.
type Mode string

const (
	// ModeInteractive shows the full TUI for manual selection.
	ModeInteractive Mode = "interactive"

	// ModeConfirm shows the pre-configured clone and requires confirmation.
	ModeConfirm Mode = "confirm"

	// ModeAuto runs the clone automatically without user interaction.
	ModeAuto Mode = "auto"
)

// OnComplete determines what happens after a successful clone.
type OnComplete string

const (
	OnCompletePrompt   OnComplete = "prompt"
	OnCompleteShutdown OnComplete = "shutdown"
	OnCompleteReboot   OnComplete = "reboot"
)

// DriveSpec specifies how to identify a drive.
type DriveSpec struct {
	// Type is how to match the drive: "internal", "boot-drive", "serial", "model", "path"
	Type string

	// Value is the match value (serial number, model substring, or device path)
	Value string
}

// Config represents the DriveSync configuration.
type Config struct {
	// Mode determines operation mode
	Mode Mode

	// Source specifies the source drive
	Source *DriveSpec

	// Destination specifies the destination drive
	Destination *DriveSpec

	// Verify enables read-back verification
	Verify bool

	// OnComplete action after successful clone
	OnComplete OnComplete

	// BlockSize in bytes (default 64KB)
	BlockSize int

	// DirectIO enables O_DIRECT (default true)
	DirectIO bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Mode:       ModeInteractive,
		Verify:     false,
		OnComplete: OnCompletePrompt,
		BlockSize:  64 * 1024,
		DirectIO:   true,
	}
}

// ConfigPaths returns the paths to search for configuration files.
func ConfigPaths() []string {
	return []string{
		"/drivesync.kdl",           // Root of boot drive
		"/boot/drivesync.kdl",      // Boot partition
		"/etc/drivesync.kdl",       // System config
		"./drivesync.kdl",          // Current directory (for testing)
	}
}

// Load searches for and loads the configuration file.
// Returns default config if no file is found.
func Load() (*Config, string, error) {
	for _, path := range ConfigPaths() {
		if _, err := os.Stat(path); err == nil {
			cfg, err := LoadFromFile(path)
			if err != nil {
				return nil, path, fmt.Errorf("error loading %s: %w", path, err)
			}
			return cfg, path, nil
		}
	}

	// No config file found, use defaults
	return DefaultConfig(), "", nil
}

// LoadFromFile loads configuration from a specific file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return Parse(data)
}

// Parse parses KDL configuration data.
func Parse(data []byte) (*Config, error) {
	doc, err := kdl.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("KDL parse error: %w", err)
	}

	cfg := DefaultConfig()

	for _, node := range doc.Nodes {
		nodeName := node.Name.ValueString()
		switch nodeName {
		case "mode":
			if len(node.Arguments) > 0 {
				mode := Mode(node.Arguments[0].ValueString())
				switch mode {
				case ModeInteractive, ModeConfirm, ModeAuto:
					cfg.Mode = mode
				default:
					return nil, fmt.Errorf("invalid mode: %s", mode)
				}
			}

		case "source":
			spec, err := parseDriveSpec(node)
			if err != nil {
				return nil, fmt.Errorf("source: %w", err)
			}
			cfg.Source = spec

		case "destination":
			spec, err := parseDriveSpec(node)
			if err != nil {
				return nil, fmt.Errorf("destination: %w", err)
			}
			cfg.Destination = spec

		case "verify":
			if len(node.Arguments) > 0 {
				cfg.Verify = toBool(node.Arguments[0].Value)
			} else {
				cfg.Verify = true
			}

		case "on-complete":
			if len(node.Arguments) > 0 {
				action := OnComplete(node.Arguments[0].ValueString())
				switch action {
				case OnCompletePrompt, OnCompleteShutdown, OnCompleteReboot:
					cfg.OnComplete = action
				default:
					return nil, fmt.Errorf("invalid on-complete: %s", action)
				}
			}

		case "block-size":
			if len(node.Arguments) > 0 {
				if size, ok := toInt(node.Arguments[0].Value); ok {
					cfg.BlockSize = size
				}
			}

		case "direct-io":
			if len(node.Arguments) > 0 {
				cfg.DirectIO = toBool(node.Arguments[0].Value)
			} else {
				cfg.DirectIO = true
			}
		}
	}

	return cfg, nil
}

// parseDriveSpec parses a drive specification node.
func parseDriveSpec(node *document.Node) (*DriveSpec, error) {
	spec := &DriveSpec{}

	// Check for shorthand: source "internal" or destination "boot-drive"
	if len(node.Arguments) > 0 {
		arg := node.Arguments[0].ValueString()
		switch arg {
		case "internal":
			spec.Type = "internal"
			return spec, nil
		case "boot-drive":
			spec.Type = "boot-drive"
			return spec, nil
		default:
			// Assume it's a path if it starts with /dev/
			if strings.HasPrefix(arg, "/dev/") {
				spec.Type = "path"
				spec.Value = arg
				return spec, nil
			}
			return nil, fmt.Errorf("invalid drive spec: %s", arg)
		}
	}

	// Check properties
	if props := node.Properties; props != nil {
		if serial, ok := props["serial"]; ok {
			spec.Type = "serial"
			spec.Value = serial.ValueString()
		} else if model, ok := props["model"]; ok {
			spec.Type = "model"
			spec.Value = model.ValueString()
		} else if path, ok := props["path"]; ok {
			spec.Type = "path"
			spec.Value = path.ValueString()
		}
	}

	if spec.Type == "" {
		return nil, fmt.Errorf("no drive specification provided")
	}

	return spec, nil
}

func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "yes" || val == "1"
	default:
		return false
	}
}

func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

// IsPreConfigured returns true if the config specifies source and destination.
func (c *Config) IsPreConfigured() bool {
	return c.Source != nil && c.Destination != nil
}

// IsSelfOverwrite returns true if destination is the boot drive.
func (c *Config) IsSelfOverwrite() bool {
	return c.Destination != nil && c.Destination.Type == "boot-drive"
}

// WriteExample writes an example configuration file.
func WriteExample(path string) error {
	example := `// DriveSync Configuration
// Place this file at the root of your boot drive as drivesync.kdl

// Operation mode:
//   "interactive" - Full TUI, manual selection (default)
//   "confirm"     - Show pre-configured clone, require Enter to start
//   "auto"        - Clone automatically on boot, no interaction
mode "confirm"

// Source drive specification:
//   "internal"           - First non-USB, non-removable drive with Windows
//   serial="ABC123"      - Match by serial number
//   model="Samsung"      - Match by model name (substring)
//   path="/dev/nvme0n1"  - Exact device path
source "internal"

// Destination drive specification:
//   "boot-drive"         - The drive we booted from (self-overwrite mode)
//   serial="XYZ789"      - Match by serial number
//   model="WD_BLACK"     - Match by model name (substring)
//   path="/dev/sda"      - Exact device path
destination "boot-drive"

// Verify clone with read-back comparison (slower but safer)
verify false

// Action after successful clone:
//   "prompt"   - Show completion screen, wait for user (default)
//   "shutdown" - Power off immediately
//   "reboot"   - Reboot immediately
on-complete "prompt"

// Advanced options
// block-size 65536    // Block size in bytes (default 64KB)
// direct-io true      // Use O_DIRECT to bypass cache (default true)
`

	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return os.WriteFile(path, []byte(example), 0644)
}
