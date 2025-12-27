package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/standardbeagle/drivesync/internal/devices"
)

// Styles
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dangerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Screen represents the current view
type Screen int

const (
	ScreenElevate Screen = iota
	ScreenSelectDisk
	ScreenConfirmClear
	ScreenProgress
	ScreenComplete
	ScreenRestore
	ScreenError
)

// BackupSize is how much to backup from start and end of disk
const BackupSize = 1024 * 1024 // 1MB

// Model is the Bubble Tea model
type Model struct {
	screen       Screen
	disks        []*devices.Disk
	cursor       int
	selectedDisk *devices.Disk
	confirmInput string
	message      string
	errorMsg     string
	width        int
	height       int
	quitting     bool
}

func newModel() Model {
	m := Model{
		screen: ScreenSelectDisk,
		width:  80,
		height: 24,
	}

	// Check if we need elevation
	if !isAdmin() {
		m.screen = ScreenElevate
	}

	return m
}

func (m Model) Init() tea.Cmd {
	if m.screen == ScreenElevate {
		return nil
	}
	return m.loadDisks
}

type disksLoadedMsg struct {
	disks []*devices.Disk
}

type errorMsg struct {
	err error
}

type operationCompleteMsg struct {
	message string
}

func (m Model) loadDisks() tea.Msg {
	disks, err := devices.Enumerate()
	if err != nil {
		return errorMsg{err}
	}

	// Populate partition info
	devices.PopulatePartitionInfo(disks)

	return disksLoadedMsg{disks}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case disksLoadedMsg:
		m.disks = msg.disks
		return m, nil

	case startClearMsg:
		return m, m.doClear

	case errorMsg:
		m.screen = ScreenError
		m.errorMsg = msg.err.Error()
		return m, nil

	case operationCompleteMsg:
		m.screen = ScreenComplete
		m.message = msg.message
		return m, nil
	}

	return m, nil
}

type elevateMsg struct{}
type startClearMsg struct{}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Handle confirm screen input first (before other key handlers steal keys)
	if m.screen == ScreenConfirmClear {
		switch msg.Type {
		case tea.KeyEsc:
			return m.handleEscape()
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyBackspace:
			if len(m.confirmInput) > 0 {
				m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
			}
		case tea.KeyEnter:
			if strings.ToUpper(m.confirmInput) == "CLEAR" {
				return m.clearDisk()
			}
		case tea.KeyRunes:
			for _, r := range msg.Runes {
				// Q/q quits from confirm screen
				if r == 'q' || r == 'Q' {
					m.quitting = true
					return m, tea.Quit
				}
				m.confirmInput += strings.ToUpper(string(r))
			}
		}
		return m, nil
	}

	switch key {
	case "ctrl+c", "q", "Q":
		if m.screen != ScreenProgress {
			m.quitting = true
			return m, tea.Quit
		}

	case "enter":
		if m.screen == ScreenElevate {
			return m, func() tea.Msg {
				err := elevate()
				if err != nil {
					return errorMsg{err}
				}
				return elevateMsg{}
			}
		}
		return m.handleEnter()

	case "up", "k":
		if m.screen == ScreenSelectDisk && m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.screen == ScreenSelectDisk && m.cursor < len(m.disks)-1 {
			m.cursor++
		}

	case "esc":
		return m.handleEscape()

	case "r", "R":
		if m.screen == ScreenSelectDisk {
			m.screen = ScreenRestore
		}
	}

	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenSelectDisk:
		if len(m.disks) > 0 && m.cursor < len(m.disks) {
			m.selectedDisk = m.disks[m.cursor]
			m.screen = ScreenConfirmClear
			m.confirmInput = ""
		}

	case ScreenComplete, ScreenError:
		m = newModel()
		return m, m.loadDisks
	}

	return m, nil
}

func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenConfirmClear, ScreenRestore:
		m.screen = ScreenSelectDisk
		m.confirmInput = ""
	}
	return m, nil
}

func (m Model) clearDisk() (tea.Model, tea.Cmd) {
	m.screen = ScreenProgress
	m.message = "Clearing partition table..."

	// Return a tick that triggers the actual work after UI renders
	return m, tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return startClearMsg{}
	})
}

func (m Model) doClear() tea.Msg {
	disk := m.selectedDisk

	// Create backup first
	backupDir := filepath.Join(os.TempDir(), "diskclear-backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return errorMsg{fmt.Errorf("cannot create backup directory: %w", err)}
	}

	timestamp := time.Now().Format("20060102-150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s-%s.bin", disk.Name, timestamp))

	// Open disk
	f, err := os.OpenFile(disk.Path, os.O_RDWR, 0)
	if err != nil {
		return errorMsg{fmt.Errorf("cannot open disk: %w (try running as root/admin)", err)}
	}
	defer f.Close()

	// Read and backup first 1MB
	startData := make([]byte, BackupSize)
	n, err := f.Read(startData)
	if err != nil {
		return errorMsg{fmt.Errorf("cannot read disk start: %w", err)}
	}
	startData = startData[:n]

	// Read backup GPT at end (if disk is large enough)
	var endData []byte
	if disk.SizeBytes > BackupSize*2 {
		endData = make([]byte, BackupSize)
		_, err := f.Seek(-BackupSize, 2) // 2 = from end
		if err == nil {
			n, _ := f.Read(endData)
			endData = endData[:n]
		}
	}

	// Write backup file
	backup, err := os.Create(backupFile)
	if err != nil {
		return errorMsg{fmt.Errorf("cannot create backup: %w", err)}
	}

	// Write header with metadata
	header := fmt.Sprintf("DISKCLEAR-BACKUP\nDevice: %s\nModel: %s\nSize: %d\nTimestamp: %s\nStartLen: %d\nEndLen: %d\n---DATA---\n",
		disk.Path, disk.Model, disk.SizeBytes, timestamp, len(startData), len(endData))
	if _, err = backup.WriteString(header); err != nil {
		backup.Close()
		return errorMsg{fmt.Errorf("cannot write backup header: %w", err)}
	}
	if _, err = backup.Write(startData); err != nil {
		backup.Close()
		return errorMsg{fmt.Errorf("cannot write backup data: %w", err)}
	}
	if _, err = backup.Write(endData); err != nil {
		backup.Close()
		return errorMsg{fmt.Errorf("cannot write backup end data: %w", err)}
	}
	backup.Close()

	// Now clear the partition table
	// Zero first 1MB (MBR + GPT header + partition entries)
	zeros := make([]byte, BackupSize)
	_, err = f.Seek(0, 0)
	if err != nil {
		return errorMsg{fmt.Errorf("cannot seek to start: %w", err)}
	}

	_, err = f.Write(zeros)
	if err != nil {
		return errorMsg{fmt.Errorf("cannot write zeros: %w", err)}
	}

	// Zero last 1MB (backup GPT)
	if disk.SizeBytes > BackupSize*2 {
		_, err = f.Seek(-BackupSize, 2)
		if err == nil {
			if _, err = f.Write(zeros); err != nil {
				return errorMsg{fmt.Errorf("cannot zero backup GPT: %w", err)}
			}
		}
	}

	if err = f.Sync(); err != nil {
		return errorMsg{fmt.Errorf("cannot sync disk: %w", err)}
	}

	return operationCompleteMsg{
		message: fmt.Sprintf("Partition table cleared!\n\nBackup saved to:\n%s\n\nThe drive should now appear as uninitialized.", backupFile),
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var content string

	switch m.screen {
	case ScreenElevate:
		content = m.viewElevate()
	case ScreenSelectDisk:
		content = m.viewSelectDisk()
	case ScreenConfirmClear:
		content = m.viewConfirmClear()
	case ScreenProgress:
		content = m.viewProgress()
	case ScreenComplete:
		content = m.viewComplete()
	case ScreenRestore:
		content = m.viewRestore()
	case ScreenError:
		content = m.viewError()
	}

	return content
}

func (m Model) viewElevate() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(titleStyle.Render("║       DISKCLEAR - Partition Nuker    ║") + "\n")
	b.WriteString(titleStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	b.WriteString(dangerStyle.Render("  ⚠ Elevation Required") + "\n\n")
	b.WriteString(infoStyle.Render("  "+strings.ReplaceAll(getElevatePrompt(), "\n", "\n  ")) + "\n\n")

	b.WriteString(dimStyle.Render("  Enter Elevate   Q Quit") + "\n")

	return b.String()
}

func (m Model) viewSelectDisk() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(titleStyle.Render("║       DISKCLEAR - Partition Nuker    ║") + "\n")
	b.WriteString(titleStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	b.WriteString(infoStyle.Render("  Select a disk to clear its partition table:") + "\n\n")

	if len(m.disks) == 0 {
		b.WriteString(dimStyle.Render("  No disks found. Try running as root/admin.") + "\n")
	}

	for i, disk := range m.disks {
		cursor := "  "
		style := normalStyle
		if i == m.cursor {
			cursor = "▶ "
			style = selectedStyle
		}

		line := fmt.Sprintf("%s[%s] %s %.1f GB",
			cursor,
			disk.Name,
			disk.DisplayName(),
			disk.SizeGB(),
		)

		b.WriteString(style.Render(line) + "\n")

		// Show partitions
		for j, part := range disk.Partitions {
			tree := "    ├─ "
			if j == len(disk.Partitions)-1 {
				tree = "    └─ "
			}
			partInfo := fmt.Sprintf("%s%s: %s", tree, part.Name, part.TypeName)
			if part.FSType != "" {
				partInfo += fmt.Sprintf(" (%s)", devices.NormalizeFSType(part.FSType))
			}
			b.WriteString(dimStyle.Render(partInfo) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓ Select   Enter Confirm   R Restore   Q Quit") + "\n")

	return b.String()
}

func (m Model) viewConfirmClear() string {
	var b strings.Builder

	b.WriteString(dangerStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(dangerStyle.Render("║          ⚠ WARNING ⚠                ║") + "\n")
	b.WriteString(dangerStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	b.WriteString("  You are about to clear the partition table on:\n\n")
	b.WriteString(dangerStyle.Render(fmt.Sprintf("    %s - %s (%.1f GB)\n\n",
		m.selectedDisk.Path,
		m.selectedDisk.DisplayName(),
		m.selectedDisk.SizeGB(),
	)))

	b.WriteString("  This will:\n")
	b.WriteString("    • Backup partition data first (can be restored)\n")
	b.WriteString("    • Zero out MBR and GPT partition tables\n")
	b.WriteString("    • Make the disk appear as uninitialized\n")
	b.WriteString("    • NOT erase actual file data\n\n")

	// Show what will be wiped
	b.WriteString(infoStyle.Render("  Current partitions that will be removed:") + "\n")
	for _, part := range m.selectedDisk.Partitions {
		b.WriteString(fmt.Sprintf("    • %s: %s (%.1f GB)\n",
			part.Name, part.TypeName, part.SizeGB()))
	}

	b.WriteString("\n")
	b.WriteString(dangerStyle.Render("  Type CLEAR to confirm: ") + m.confirmInput + "█\n\n")
	b.WriteString(dimStyle.Render("  Esc Cancel") + "\n")

	return b.String()
}

func (m Model) viewProgress() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(titleStyle.Render("║           Working...                 ║") + "\n")
	b.WriteString(titleStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	b.WriteString(infoStyle.Render("  "+m.message) + "\n")

	return b.String()
}

func (m Model) viewComplete() string {
	var b strings.Builder

	b.WriteString(successStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(successStyle.Render("║           ✓ Complete                 ║") + "\n")
	b.WriteString(successStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	b.WriteString(successStyle.Render("  "+strings.ReplaceAll(m.message, "\n", "\n  ")) + "\n\n")

	b.WriteString(dimStyle.Render("  Press Enter to continue, Q to quit") + "\n")

	return b.String()
}

func (m Model) viewRestore() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(titleStyle.Render("║           Restore Backup             ║") + "\n")
	b.WriteString(titleStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	backupDir := filepath.Join(os.TempDir(), "diskclear-backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) == 0 {
		b.WriteString(dimStyle.Render("  No backups found in:\n  "+backupDir) + "\n\n")
		b.WriteString(dimStyle.Render("  Esc Back") + "\n")
		return b.String()
	}

	b.WriteString(infoStyle.Render("  Available backups:") + "\n\n")
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".bin") {
			b.WriteString(fmt.Sprintf("    • %s\n", entry.Name()))
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  To restore, run:") + "\n")
	b.WriteString(dimStyle.Render("    diskclear --restore <backup.bin> /dev/sdX") + "\n\n")
	b.WriteString(dimStyle.Render("  Esc Back") + "\n")

	return b.String()
}

func (m Model) viewError() string {
	var b strings.Builder

	b.WriteString(dangerStyle.Render("╔══════════════════════════════════════╗") + "\n")
	b.WriteString(dangerStyle.Render("║           ✗ Error                    ║") + "\n")
	b.WriteString(dangerStyle.Render("╚══════════════════════════════════════╝") + "\n\n")

	b.WriteString(dangerStyle.Render("  "+m.errorMsg) + "\n\n")

	b.WriteString(dimStyle.Render("  Press Enter to retry, Q to quit") + "\n")

	return b.String()
}

func main() {
	// Check for restore mode
	if len(os.Args) >= 4 && os.Args[1] == "--restore" {
		restoreBackup(os.Args[2], os.Args[3])
		return
	}

	p := tea.NewProgram(
		newModel(),
		tea.WithAltScreen(),
		tea.WithInputTTY(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func restoreBackup(backupPath, devicePath string) {
	fmt.Printf("Restoring %s to %s...\n", backupPath, devicePath)

	// Read backup file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading backup: %v\n", err)
		os.Exit(1)
	}

	// Find the data section
	marker := []byte("---DATA---\n")
	idx := strings.Index(string(data), string(marker))
	if idx == -1 {
		fmt.Fprintf(os.Stderr, "Invalid backup file format\n")
		os.Exit(1)
	}

	// Parse header for lengths
	header := string(data[:idx])
	var startLen, endLen int
	for _, line := range strings.Split(header, "\n") {
		if strings.HasPrefix(line, "StartLen: ") {
			_, _ = fmt.Sscanf(line, "StartLen: %d", &startLen)
		}
		if strings.HasPrefix(line, "EndLen: ") {
			_, _ = fmt.Sscanf(line, "EndLen: %d", &endLen)
		}
	}

	binaryData := data[idx+len(marker):]

	if startLen == 0 || len(binaryData) < startLen {
		fmt.Fprintf(os.Stderr, "Backup file corrupted\n")
		os.Exit(1)
	}

	startData := binaryData[:startLen]
	var endData []byte
	if endLen > 0 && len(binaryData) >= startLen+endLen {
		endData = binaryData[startLen : startLen+endLen]
	}

	// Show what we're about to do
	fmt.Printf("Backup header:\n%s\n", header)
	fmt.Printf("Start data: %d bytes\n", len(startData))
	fmt.Printf("End data: %d bytes\n", len(endData))
	fmt.Printf("\nFirst 64 bytes:\n%s\n", hex.Dump(startData[:min(64, len(startData))]))

	fmt.Printf("Type 'yes' to restore: ")
	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Println("Aborted")
		return
	}

	// Open and write
	f, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open device: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	_, err = f.Write(startData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing start data: %v\n", err)
		os.Exit(1)
	}

	if len(endData) > 0 {
		_, err = f.Seek(-int64(len(endData)), 2)
		if err == nil {
			if _, err = f.Write(endData); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing end data: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if err = f.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "Error syncing disk: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Restore complete!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
