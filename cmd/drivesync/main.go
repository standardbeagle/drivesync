package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/beagle/drivesync/internal/config"
	"github.com/beagle/drivesync/internal/tui"
)

var version = "dev"

func main() {
	// Parse flags
	showVersion := flag.Bool("version", false, "Show version")
	showHelp := flag.Bool("help", false, "Show help")
	configFile := flag.String("config", "", "Path to configuration file")
	writeExample := flag.String("write-example-config", "", "Write example config to specified path")
	flag.Parse()

	if *showVersion {
		fmt.Printf("drivesync %s\n", version)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	if *writeExample != "" {
		if err := config.WriteExample(*writeExample); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing example config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Example configuration written to: %s\n", *writeExample)
		os.Exit(0)
	}

	// Check for root
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: drivesync requires root privileges")
		fmt.Fprintln(os.Stderr, "Run with: sudo drivesync")
		os.Exit(1)
	}

	// Load configuration
	var cfg *config.Config
	var cfgPath string
	var err error

	if *configFile != "" {
		cfg, err = config.LoadFromFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config %s: %v\n", *configFile, err)
			os.Exit(1)
		}
		cfgPath = *configFile
		fmt.Printf("Loaded configuration from: %s\n", cfgPath)
	} else {
		cfg, cfgPath, err = config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		if cfgPath != "" {
			fmt.Printf("Loaded configuration from: %s\n", cfgPath)
		}
	}

	// Run TUI
	if err := tui.RunWithConfig(cfg, cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`DriveSync - Block-level drive cloning utility

Usage: drivesync [options]

Options:
  --version                  Show version
  --help                     Show this help
  --config <file>            Path to configuration file (KDL format)
  --write-example-config <file>  Write example config to file

Configuration:
  DriveSync looks for configuration in these locations:
    /drivesync.kdl           Root of boot drive
    /boot/drivesync.kdl      Boot partition
    /etc/drivesync.kdl       System config
    ./drivesync.kdl          Current directory

  Configuration allows pre-setting source and destination drives
  for one-click or automatic cloning. Modes:
    interactive  Full TUI, manual selection (default)
    confirm      Show pre-configured clone, Enter to start
    auto         Clone automatically on boot

Self-Overwrite Mode:
  When booted from the destination drive (via USB), DriveSync can clone
  the internal drive onto itself, running entirely from RAM.

  To prepare a self-overwrite drive:
    1. Format drive as FAT32
    2. Extract DriveSync boot files
    3. Create drivesync.kdl with:
         mode "confirm"
         source "internal"
         destination "boot-drive"

For more information, visit: https://github.com/beagle/drivesync`)
}
