package config

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(*Config) error
		wantErr bool
	}{
		{
			name:  "empty config uses defaults",
			input: "",
			check: func(c *Config) error {
				if c.Mode != ModeInteractive {
					t.Errorf("expected interactive mode, got %s", c.Mode)
				}
				return nil
			},
		},
		{
			name:  "mode auto",
			input: `mode "auto"`,
			check: func(c *Config) error {
				if c.Mode != ModeAuto {
					t.Errorf("expected auto mode, got %s", c.Mode)
				}
				return nil
			},
		},
		{
			name:  "mode confirm",
			input: `mode "confirm"`,
			check: func(c *Config) error {
				if c.Mode != ModeConfirm {
					t.Errorf("expected confirm mode, got %s", c.Mode)
				}
				return nil
			},
		},
		{
			name:    "invalid mode",
			input:   `mode "invalid"`,
			wantErr: true,
		},
		{
			name:  "source internal",
			input: `source "internal"`,
			check: func(c *Config) error {
				if c.Source == nil {
					t.Error("expected source to be set")
					return nil
				}
				if c.Source.Type != "internal" {
					t.Errorf("expected internal, got %s", c.Source.Type)
				}
				return nil
			},
		},
		{
			name:  "destination boot-drive",
			input: `destination "boot-drive"`,
			check: func(c *Config) error {
				if c.Destination == nil {
					t.Error("expected destination to be set")
					return nil
				}
				if c.Destination.Type != "boot-drive" {
					t.Errorf("expected boot-drive, got %s", c.Destination.Type)
				}
				return nil
			},
		},
		{
			name:  "source with serial",
			input: `source serial="ABC123"`,
			check: func(c *Config) error {
				if c.Source == nil {
					t.Error("expected source to be set")
					return nil
				}
				if c.Source.Type != "serial" {
					t.Errorf("expected serial, got %s", c.Source.Type)
				}
				if c.Source.Value != "ABC123" {
					t.Errorf("expected ABC123, got %s", c.Source.Value)
				}
				return nil
			},
		},
		{
			name:  "destination with model",
			input: `destination model="Samsung"`,
			check: func(c *Config) error {
				if c.Destination == nil {
					t.Error("expected destination to be set")
					return nil
				}
				if c.Destination.Type != "model" {
					t.Errorf("expected model, got %s", c.Destination.Type)
				}
				if c.Destination.Value != "Samsung" {
					t.Errorf("expected Samsung, got %s", c.Destination.Value)
				}
				return nil
			},
		},
		{
			name:  "source with path",
			input: `source "/dev/nvme0n1"`,
			check: func(c *Config) error {
				if c.Source == nil {
					t.Error("expected source to be set")
					return nil
				}
				if c.Source.Type != "path" {
					t.Errorf("expected path, got %s", c.Source.Type)
				}
				if c.Source.Value != "/dev/nvme0n1" {
					t.Errorf("expected /dev/nvme0n1, got %s", c.Source.Value)
				}
				return nil
			},
		},
		{
			name:  "verify true",
			input: `verify true`,
			check: func(c *Config) error {
				if !c.Verify {
					t.Error("expected verify to be true")
				}
				return nil
			},
		},
		{
			name:  "verify without argument",
			input: `verify`,
			check: func(c *Config) error {
				if !c.Verify {
					t.Error("expected verify to be true")
				}
				return nil
			},
		},
		{
			name:  "on-complete shutdown",
			input: `on-complete "shutdown"`,
			check: func(c *Config) error {
				if c.OnComplete != OnCompleteShutdown {
					t.Errorf("expected shutdown, got %s", c.OnComplete)
				}
				return nil
			},
		},
		{
			name:  "on-complete reboot",
			input: `on-complete "reboot"`,
			check: func(c *Config) error {
				if c.OnComplete != OnCompleteReboot {
					t.Errorf("expected reboot, got %s", c.OnComplete)
				}
				return nil
			},
		},
		{
			name:    "invalid on-complete",
			input:   `on-complete "invalid"`,
			wantErr: true,
		},
		{
			name: "full config",
			input: `
mode "confirm"
source "internal"
destination "boot-drive"
verify true
on-complete "shutdown"
`,
			check: func(c *Config) error {
				if c.Mode != ModeConfirm {
					t.Errorf("mode: got %s", c.Mode)
				}
				if c.Source == nil || c.Source.Type != "internal" {
					t.Error("source not internal")
				}
				if c.Destination == nil || c.Destination.Type != "boot-drive" {
					t.Error("destination not boot-drive")
				}
				if !c.Verify {
					t.Error("verify not true")
				}
				if c.OnComplete != OnCompleteShutdown {
					t.Errorf("on-complete: got %s", c.OnComplete)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil && err == nil {
				tt.check(cfg)
			}
		})
	}
}

func TestConfigIsPreConfigured(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "default config",
			config: DefaultConfig(),
			want:   false,
		},
		{
			name: "source only",
			config: &Config{
				Source: &DriveSpec{Type: "internal"},
			},
			want: false,
		},
		{
			name: "destination only",
			config: &Config{
				Destination: &DriveSpec{Type: "boot-drive"},
			},
			want: false,
		},
		{
			name: "both set",
			config: &Config{
				Source:      &DriveSpec{Type: "internal"},
				Destination: &DriveSpec{Type: "boot-drive"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsPreConfigured(); got != tt.want {
				t.Errorf("IsPreConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigIsSelfOverwrite(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "no destination",
			config: &Config{},
			want:   false,
		},
		{
			name: "destination is boot-drive",
			config: &Config{
				Destination: &DriveSpec{Type: "boot-drive"},
			},
			want: true,
		},
		{
			name: "destination is path",
			config: &Config{
				Destination: &DriveSpec{Type: "path", Value: "/dev/sda"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsSelfOverwrite(); got != tt.want {
				t.Errorf("IsSelfOverwrite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Mode != ModeInteractive {
		t.Errorf("Mode: got %s, want interactive", cfg.Mode)
	}
	if cfg.Verify {
		t.Error("Verify should be false by default")
	}
	if cfg.OnComplete != OnCompletePrompt {
		t.Errorf("OnComplete: got %s, want prompt", cfg.OnComplete)
	}
	if cfg.BlockSize != 64*1024 {
		t.Errorf("BlockSize: got %d, want %d", cfg.BlockSize, 64*1024)
	}
	if !cfg.DirectIO {
		t.Error("DirectIO should be true by default")
	}
}
